package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

// Event is a row in the events table.
type Event struct {
	ID         int64
	DType      string
	Title      string
	Contents   string
	SortDate   string
	GroupID    int64
	GroupName  string
	AuthorName string
	RawJSON    string
	SyncedAt   string // set by DB default
}

// ChatMessage is a row in the chat_messages table.
type ChatMessage struct {
	ID           int64
	ChatroomID   int64
	ChatroomName string
	SenderName   string
	Contents     string
	SentAt       string
	RawJSON      string
	SyncedAt     string
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    dtype TEXT NOT NULL,
    title TEXT,
    contents TEXT,
    sort_date TEXT,
    group_id INTEGER,
    raw_json TEXT,
    synced_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS chat_messages (
    id INTEGER PRIMARY KEY,
    chatroom_id INTEGER NOT NULL,
    sender_name TEXT,
    contents TEXT,
    sent_at TEXT,
    raw_json TEXT,
    synced_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// migrations adds columns to existing tables. Each statement is run
// individually; "duplicate column" errors are silently ignored so
// migrations are idempotent.
var migrations = []string{
	"ALTER TABLE events ADD COLUMN group_name TEXT DEFAULT ''",
	"ALTER TABLE events ADD COLUMN author_name TEXT DEFAULT ''",
	"ALTER TABLE chat_messages ADD COLUMN chatroom_name TEXT DEFAULT ''",
}

// Open opens (or creates) the SQLite database and ensures the schema exists.
func Open(dsn string, logger *log.Logger) (*Store, error) {
	if logger != nil {
		logger.Printf("opening database %s", dsn)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// "duplicate column name" means migration already applied
			if !isDuplicateColumn(err) {
				db.Close()
				return nil, fmt.Errorf("migration %q: %w", m, err)
			}
		}
	}
	return &Store{db: db}, nil
}

func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// ResetCache deletes all events, chat messages, and the last_sync timestamp.
func (s *Store) ResetCache() error {
	_, err := s.db.Exec("DELETE FROM events; DELETE FROM chat_messages; DELETE FROM config WHERE key = 'last_sync'")
	return err
}

// SetConfig stores a key-value pair in the config table.
func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// GetConfig retrieves a config value by key.
func (s *Store) GetConfig(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", fmt.Errorf("config key %q: %w", key, err)
	}
	return val, nil
}

// UpsertEvent inserts an event, ignoring duplicates by ID.
// Returns true if the row was newly inserted, false if it already existed.
func (s *Store) UpsertEvent(e Event) (inserted bool, err error) {
	res, err := s.db.Exec(
		"INSERT OR IGNORE INTO events (id, dtype, title, contents, sort_date, group_id, group_name, author_name, raw_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		e.ID, e.DType, e.Title, e.Contents, e.SortDate, e.GroupID, e.GroupName, e.AuthorName, e.RawJSON,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpsertChatMessage inserts a chat message, ignoring duplicates by ID.
// Returns true if the row was newly inserted, false if it already existed.
func (s *Store) UpsertChatMessage(m ChatMessage) (inserted bool, err error) {
	res, err := s.db.Exec(
		"INSERT OR IGNORE INTO chat_messages (id, chatroom_id, chatroom_name, sender_name, contents, sent_at, raw_json) VALUES (?, ?, ?, ?, ?, ?, ?)",
		m.ID, m.ChatroomID, m.ChatroomName, m.SenderName, m.Contents, m.SentAt, m.RawJSON,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetLatestAnnouncements returns the most recent announcement per group, newest first.
func (s *Store) GetLatestAnnouncements() ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.dtype, e.title, e.contents, e.sort_date, e.group_id, e.group_name, e.author_name, e.synced_at
		FROM events e
		WHERE e.dtype LIKE '%Announcement%'
		AND e.sort_date = (
			SELECT MAX(e2.sort_date) FROM events e2
			WHERE e2.group_id = e.group_id AND e2.dtype LIKE '%Announcement%'
		)
		ORDER BY e.sort_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.DType, &e.Title, &e.Contents, &e.SortDate, &e.GroupID, &e.GroupName, &e.AuthorName, &e.SyncedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetLatestChatMessagePerRoom returns the most recent message per chatroom, newest first.
func (s *Store) GetLatestChatMessagePerRoom() ([]ChatMessage, error) {
	rows, err := s.db.Query(`
		SELECT cm.id, cm.chatroom_id, cm.chatroom_name, cm.sender_name, cm.contents, cm.sent_at, cm.synced_at
		FROM chat_messages cm
		WHERE cm.sent_at = (
			SELECT MAX(cm2.sent_at) FROM chat_messages cm2
			WHERE cm2.chatroom_id = cm.chatroom_id
		)
		ORDER BY cm.sent_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ChatroomID, &m.ChatroomName, &m.SenderName, &m.Contents, &m.SentAt, &m.SyncedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetNewEvents returns events synced after the given timestamp.
func (s *Store) GetNewEvents(since string) ([]Event, error) {
	rows, err := s.db.Query(
		"SELECT id, dtype, title, contents, sort_date, group_id, group_name, author_name, synced_at FROM events WHERE synced_at > ? ORDER BY sort_date",
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.DType, &e.Title, &e.Contents, &e.SortDate, &e.GroupID, &e.GroupName, &e.AuthorName, &e.SyncedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetEventsSince returns events with sort_date after the given timestamp.
func (s *Store) GetEventsSince(since string) ([]Event, error) {
	rows, err := s.db.Query(
		"SELECT id, dtype, title, contents, sort_date, group_id, group_name, author_name, synced_at FROM events WHERE sort_date > ? ORDER BY sort_date",
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.DType, &e.Title, &e.Contents, &e.SortDate, &e.GroupID, &e.GroupName, &e.AuthorName, &e.SyncedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetChatMessagesSince returns chat messages with sent_at after the given timestamp.
func (s *Store) GetChatMessagesSince(since string) ([]ChatMessage, error) {
	rows, err := s.db.Query(
		"SELECT id, chatroom_id, chatroom_name, sender_name, contents, sent_at, synced_at FROM chat_messages WHERE sent_at > ? ORDER BY sent_at",
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ChatroomID, &m.ChatroomName, &m.SenderName, &m.Contents, &m.SentAt, &m.SyncedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetNewChatMessages returns chat messages synced after the given timestamp.
func (s *Store) GetNewChatMessages(since string) ([]ChatMessage, error) {
	rows, err := s.db.Query(
		"SELECT id, chatroom_id, chatroom_name, sender_name, contents, sent_at, synced_at FROM chat_messages WHERE synced_at > ? ORDER BY sent_at",
		since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ChatroomID, &m.ChatroomName, &m.SenderName, &m.Contents, &m.SentAt, &m.SyncedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
