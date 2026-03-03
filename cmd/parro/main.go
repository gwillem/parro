package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"

	"github.com/gwillem/parro/internal/api"
	"github.com/gwillem/parro/internal/config"
	"github.com/gwillem/parro/internal/db"
)

func cacheDir(guardianID string) string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "parro", guardianID)
}

func accountDBPath(guardianID string) string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "parro", guardianID+".db")
}

func resolveAccount() (config.AccountConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.AccountConfig{}, err
	}
	if opts.Account != "" {
		acct, ok := cfg.Find(opts.Account)
		if !ok {
			return config.AccountConfig{}, fmt.Errorf("account %q not found in config", opts.Account)
		}
		return acct, nil
	}
	return cfg.Only()
}

type options struct {
	Verbose []bool   `short:"v" long:"verbose" description:"Verbose output (-v debug, -vv show last messages)"`
	Account string   `short:"a" long:"account" description:"Account to use (guardian ID or username)"`
	Login   loginCmd `command:"login" description:"Login with email and password"`
	Check   checkCmd `command:"check" description:"Fetch new messages and sync to local DB"`
	Reset   resetCmd `command:"reset" description:"Clear cached messages"`
}

type loginCmd struct {
	Args struct {
		Email    string `positional-arg-name:"user" required:"true" description:"ParnaSys email or username"`
		Password string `positional-arg-name:"pass" required:"true" description:"ParnaSys password"`
	} `positional-args:"yes"`
}

type checkCmd struct{}

type resetCmd struct{}

func (cmd *loginCmd) Execute(args []string) error {
	initVerbose()

	log.Println("Logging in...")
	tok, err := api.Login(cmd.Args.Email, cmd.Args.Password, verbose)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Fetch guardian ID from account
	client := api.NewClient(tok.RefreshToken, "")
	client.Logger = verbose
	client.SetAccessToken(tok.AccessToken)

	acct, err := client.GetAccount()
	if err != nil {
		return fmt.Errorf("fetch account: %w", err)
	}

	gid := acct.GuardianID()
	if gid == 0 {
		return fmt.Errorf("could not determine guardian ID from account")
	}
	guardianID := fmt.Sprintf("%d", gid)

	// Save credentials to config.json
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Set(config.AccountConfig{
		RefreshToken: tok.RefreshToken,
		GuardianID:   guardianID,
		Username:     cmd.Args.Email,
	})
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Create per-account DB (ensures schema exists)
	dbPath := accountDBPath(guardianID)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}
	store, err := db.Open(dbPath, verbose)
	if err != nil {
		return err
	}
	store.Close()

	log.Printf("Logged in. Guardian ID: %s", guardianID)
	log.Printf("Config: %s", config.Path())
	log.Printf("Database: %s", dbPath)
	return nil
}

func (cmd *resetCmd) Execute(args []string) error {
	initVerbose()

	acct, err := resolveAccount()
	if err != nil {
		return err
	}

	dbPath := accountDBPath(acct.GuardianID)
	store, err := db.Open(dbPath, verbose)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.ResetCache(); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	log.Println("Cache cleared. Next 'check' will do a full sync.")
	return nil
}

func (cmd *checkCmd) Execute(args []string) error {
	initVerbose()

	acct, err := resolveAccount()
	if err != nil {
		return err
	}

	dbPath := accountDBPath(acct.GuardianID)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}
	store, err := db.Open(dbPath, verbose)
	if err != nil {
		return err
	}
	defer store.Close()

	guardianID := acct.GuardianID
	attachDir := cacheDir(guardianID)

	// Detect first run (no last_sync means fresh DB)
	lastSync, err := store.GetConfig("last_sync")
	firstRun := err != nil
	if firstRun {
		lastSync = "2000-01-01T00:00:00Z"
	}
	verbose.Printf("first run: %v, last sync: %s", firstRun, lastSync)

	// Record sync start time before fetching
	syncStart := time.Now().UTC().Format(time.RFC3339)

	// Init API client and refresh token
	client := api.NewClient(acct.RefreshToken, guardianID)
	client.Logger = verbose
	if err := client.RefreshAccessToken(); err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}

	// Save the new rolling refresh token back to config.json
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	acct.RefreshToken = client.RefreshTokenValue
	cfg.Set(acct)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}

	// Fetch groups
	groups, err := client.GetGroups()
	if err != nil {
		return fmt.Errorf("fetch groups: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -30)

	// Fetch announcements per group (API returns newest first;
	// stop when older than 30 days or already-known)
	for _, g := range groups {
		gid := api.SelfID(g.Links)
		if gid == 0 {
			continue
		}
		anns, err := client.GetAnnouncements(gid)
		if err != nil {
			log.Printf("warn: announcements for group %d: %v", gid, err)
			continue
		}
		for _, a := range anns {
			if t, err := time.Parse(time.RFC3339, a.SortDate); err == nil && t.Before(cutoff) {
				break // rest are older
			}
			raw, _ := json.Marshal(a)
			author := strings.TrimSpace(a.Owner.Firstname + " " + a.Owner.Surname)
			var contents strings.Builder
			contents.WriteString(a.Contents)
			eventID := api.SelfID(a.Links)

			// Download attachments if present
			for _, att := range a.Attachments {
				if src := att.SourceEntry(); src != nil {
					destPath := attachmentPath(attachDir, eventID, src)
					if err := os.MkdirAll(attachDir, 0o755); err != nil {
						log.Printf("warn: create %s: %v", attachDir, err)
					} else if _, err := client.DownloadFile(src.URL, destPath, src.Size); err != nil {
						log.Printf("warn: download attachment: %v", err)
					} else {
						contents.WriteString(fmt.Sprintf("\n[Attachment: %s]", destPath))
					}
				}
			}

			inserted, err := store.UpsertEvent(db.Event{
				ID:         eventID,
				DType:      a.DType,
				Title:      a.Title,
				Contents:   contents.String(),
				SortDate:   a.SortDate,
				GroupID:    gid,
				GroupName:  g.Name,
				AuthorName: author,
				RawJSON:    string(raw),
			})
			if err != nil {
				log.Printf("warn: upsert announcement: %v", err)
			}
			if !inserted && !firstRun {
				break // rest are older, already stored
			}
		}
	}

	// Fetch calendar events (last 30 days)
	since := cutoff.UTC().Format(time.RFC3339)
	calEvents, err := client.GetCalendarEvents(since)
	if err != nil {
		log.Printf("warn: calendar events: %v", err)
	} else {
		for _, e := range calEvents {
			raw, _ := json.Marshal(e)
			inserted, err := store.UpsertEvent(db.Event{
				ID:       api.SelfID(e.Links),
				DType:    e.DType,
				Title:    e.Title,
				Contents: "",
				SortDate: e.SortDate,
				GroupID:  0,
				RawJSON:  string(raw),
			})
			if err != nil {
				log.Printf("warn: upsert calendar event: %v", err)
			}
			if !inserted && !firstRun {
				break
			}
		}
	}

	// Fetch chatrooms and messages (skip rooms inactive for >30 days)
	rooms, err := client.GetChatRooms()
	activeRoomIDs := map[int64]bool{}
	if err != nil {
		log.Printf("warn: chatrooms: %v", err)
	} else {
		for _, room := range rooms {
			roomID := api.SelfID(room.Links)
			if roomID == 0 {
				continue
			}
			if t, err := time.Parse(time.RFC3339, room.SortDate); err == nil && t.Before(cutoff) {
				verbose.Printf("skipping stale chatroom %d %q (last activity %s)", roomID, room.Title, room.SortDate)
				continue
			}
			activeRoomIDs[roomID] = true
			msgs, err := client.GetChatMessages(roomID)
			if err != nil {
				log.Printf("warn: chatmessages for room %d: %v", roomID, err)
				continue
			}
			for _, m := range msgs {
				raw, _ := json.Marshal(m)
				sender := strings.TrimSpace(m.Identity.Firstname + " " + m.Identity.Surname)
				contents := m.Text

				// Download attachment if present
				if m.Attachment != nil {
					if src := m.Attachment.SourceEntry(); src != nil {
						msgID := api.SelfID(m.Links)
						destPath := attachmentPath(attachDir, msgID, src)
						if err := os.MkdirAll(attachDir, 0o755); err != nil {
							log.Printf("warn: create %s: %v", attachDir, err)
						} else if _, err := client.DownloadFile(src.URL, destPath, src.Size); err != nil {
							log.Printf("warn: download attachment: %v", err)
						} else {
							contents += fmt.Sprintf("\n[Attachment: %s]", destPath)
						}
					}
				}

				inserted, err := store.UpsertChatMessage(db.ChatMessage{
					ID:           api.SelfID(m.Links),
					ChatroomID:   roomID,
					ChatroomName: room.Title,
					SenderName:   sender,
					Contents:     contents,
					SentAt:       m.LastModifiedAt,
					RawJSON:      string(raw),
				})
				if err != nil {
					log.Printf("warn: upsert chatmessage: %v", err)
				}
				if !inserted && !firstRun {
					break // rest are older, already stored
				}
			}
		}
	}

	// Save sync timestamp
	if err := store.SetConfig("last_sync", syncStart); err != nil {
		log.Printf("warn: save last_sync: %v", err)
	}

	// First run: silent seed — don't print anything
	if firstRun {
		log.Println("Initial sync complete. Run 'check' again to see new items.")
		if verbosity() >= 2 {
			return printLatestSummary(store, activeRoomIDs)
		}
		return nil
	}

	// Print new items
	newEvents, err := store.GetNewEvents(lastSync)
	if err != nil {
		return fmt.Errorf("query new events: %w", err)
	}
	newMsgs, err := store.GetNewChatMessages(lastSync)
	if err != nil {
		return fmt.Errorf("query new messages: %w", err)
	}

	if len(newEvents) == 0 && len(newMsgs) == 0 {
		log.Println("No new items.")
		if verbosity() >= 2 {
			return printLatestSummary(store, activeRoomIDs)
		}
		return nil
	}

	// Split events by type
	var announcements, calendar []db.Event
	for _, e := range newEvents {
		if strings.Contains(e.DType, "Announcement") {
			announcements = append(announcements, e)
		} else {
			calendar = append(calendar, e)
		}
	}

	if len(announcements) > 0 {
		fmt.Println("=== New Announcements ===")
		for _, a := range announcements {
			fmt.Printf("\n[%s] [%s] %s (by %s)\n%s\n", a.SortDate, a.GroupName, a.Title, a.AuthorName, a.Contents)
		}
	}

	if len(calendar) > 0 {
		fmt.Println("\n=== Upcoming Calendar Events ===")
		for _, e := range calendar {
			title := e.Title
			if title == "" {
				title = "(calendar item)"
			}
			fmt.Printf("\n[%s] %s\n", e.SortDate, title)
		}
	}

	if len(newMsgs) > 0 {
		fmt.Println("\n=== New Chat Messages ===")
		for _, m := range newMsgs {
			fmt.Printf("\n[%s] [%s] %s: %s\n", m.SentAt, m.ChatroomName, m.SenderName, m.Contents)
		}
	}

	if verbosity() >= 2 {
		if err := printLatestSummary(store, activeRoomIDs); err != nil {
			return err
		}
	}

	return nil
}

func printLatestSummary(store *db.Store, activeRoomIDs map[int64]bool) error {
	anns, err := store.GetLatestAnnouncements()
	if err != nil {
		return fmt.Errorf("query latest announcements: %w", err)
	}
	allMsgs, err := store.GetLatestChatMessagePerRoom()
	if err != nil {
		return fmt.Errorf("query latest chat messages: %w", err)
	}

	// Filter to only active (non-stale) chatrooms
	var msgs []db.ChatMessage
	for _, m := range allMsgs {
		if activeRoomIDs[m.ChatroomID] {
			msgs = append(msgs, m)
		}
	}

	if len(anns) > 0 {
		fmt.Println("\n=== Latest Announcement Per Group ===")
		for _, a := range anns {
			fmt.Printf("\n[%s] [%s] %s (by %s)\n%s\n", a.SortDate, a.GroupName, a.Title, a.AuthorName, a.Contents)
		}
	}
	if len(msgs) > 0 {
		fmt.Println("\n=== Latest Chat Message Per Room ===")
		for _, m := range msgs {
			fmt.Printf("\n[%s] [%s] %s: %s\n", m.SentAt, m.ChatroomName, m.SenderName, m.Contents)
		}
	}
	return nil
}

// attachmentPath returns the destination path for a downloaded attachment.
func attachmentPath(dir string, msgID int64, entry *api.AttachmentEntry) string {
	filename := entry.Filename
	if filename == "" {
		// Derive filename from URL (e.g. ".../1-abc123.jpeg" → "1-abc123.jpeg")
		if u, err := url.Parse(entry.URL); err == nil {
			filename = path.Base(u.Path)
		}
	}
	if filename == "" || filename == "." {
		filename = "file"
	}
	return filepath.Join(dir, fmt.Sprintf("%d_%s", msgID, filename))
}

var opts options

// verbose is the debug logger. Writes to stderr when -v is given,
// otherwise discards output. Initialized lazily via initVerbose().
var verbose *log.Logger

func verbosity() int { return len(opts.Verbose) }

func initVerbose() {
	if verbose != nil {
		return
	}
	out := io.Discard
	if verbosity() >= 1 {
		out = os.Stderr
	}
	verbose = log.New(out, "debug: ", log.Ltime)
}

func main() {
	// Log full CLI invocation to /tmp/parro.log
	logFile, err := os.OpenFile("/tmp/parro.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		fmt.Fprintf(logFile, "[%s] args: %q\n", time.Now().Format(time.RFC3339), os.Args)
		logFile.Close()
	}

	parser := flags.NewParser(&opts, flags.Default)
	parser.SubcommandsOptional = false

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}
}
