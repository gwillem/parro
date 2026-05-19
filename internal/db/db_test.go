package db

import (
	"testing"
)

func TestOpenCreatesSchema(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Verify tables exist by inserting and querying
	if err := store.SetConfig("test_key", "test_value"); err != nil {
		t.Fatal("config table not created:", err)
	}
	val, err := store.GetConfig("test_key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "test_value" {
		t.Fatalf("got %q, want %q", val, "test_value")
	}
}

func TestConfigGetMissing(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetConfig("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing config key")
	}
}

func TestUpsertEventDedup(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ev := Event{
		ID:       123,
		DType:    "event.RAnnouncementEventPrimer",
		Title:    "Test Title",
		Contents: "Hello world",
		SortDate: "2026-03-02T21:18:27.383+01:00",
		GroupID:  456,
		RawJSON:  `{"title":"Test Title"}`,
	}

	inserted, err := store.UpsertEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first insert should return inserted=true")
	}
	// Upsert same ID again — should not error, but inserted=false
	inserted, err = store.UpsertEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate insert should return inserted=false")
	}

	// Should still be exactly one row
	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM events WHERE id = ?", ev.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d rows, want 1", count)
	}
}

func TestHasEventTitle(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.UpsertEvent(Event{ID: 1, DType: "event.RCalendarItemEventPrimer", Title: "", SortDate: "2026-03-01T10:00:00+01:00", RawJSON: "{}"})
	store.UpsertEvent(Event{ID: 2, DType: "event.RCalendarItemEventPrimer", Title: "Studiedag", SortDate: "2026-03-02T10:00:00+01:00", RawJSON: "{}"})

	if store.HasEventTitle(1) {
		t.Error("HasEventTitle(1) = true, want false (empty title)")
	}
	if !store.HasEventTitle(2) {
		t.Error("HasEventTitle(2) = false, want true")
	}
	if store.HasEventTitle(999) {
		t.Error("HasEventTitle(999) = true, want false (missing row)")
	}
}

func TestUpsertEventBackfillTitle(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// First insert: no title yet (calendar list endpoint omits it).
	inserted, err := store.UpsertEvent(Event{ID: 7, DType: "event.RCalendarItemEventPrimer", Title: "", SortDate: "2026-03-01T10:00:00+01:00", RawJSON: `{"v":1}`})
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first upsert should report inserted=true")
	}
	if store.HasEventTitle(7) {
		t.Fatal("title should still be empty after first insert")
	}

	// Second upsert with a title: should backfill via the UPDATE branch.
	inserted, err = store.UpsertEvent(Event{ID: 7, DType: "event.RCalendarItemEventPrimer", Title: "Schoolreis", SortDate: "2026-03-01T10:00:00+01:00", RawJSON: `{"v":2}`})
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("second upsert should report inserted=false (already existed)")
	}
	if !store.HasEventTitle(7) {
		t.Fatal("title should be backfilled after second upsert")
	}
}

func TestUpsertChatMessageDedup(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	msg := ChatMessage{
		ID:         789,
		ChatroomID: 111,
		SenderName: "Anna Bakker",
		Contents:   "Hallo!",
		SentAt:     "2026-03-02T22:48:43.818+01:00",
		RawJSON:    `{"contents":"Hallo!"}`,
	}

	inserted, err := store.UpsertChatMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first insert should return inserted=true")
	}
	inserted, err = store.UpsertChatMessage(msg)
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Fatal("duplicate insert should return inserted=false")
	}

	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM chat_messages WHERE id = ?", msg.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got %d rows, want 1", count)
	}
}

func TestGetNewEvents(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert an event
	ev := Event{
		ID:       100,
		DType:    "event.RAnnouncementEventPrimer",
		Title:    "New Announcement",
		Contents: "Content here",
		SortDate: "2026-03-02T21:00:00.000+01:00",
		GroupID:  1,
		RawJSON:  "{}",
	}
	if _, err := store.UpsertEvent(ev); err != nil {
		t.Fatal(err)
	}

	// Query events synced since a time in the past — should find our event
	events, err := store.GetNewEvents("2000-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Title != "New Announcement" {
		t.Fatalf("got title %q", events[0].Title)
	}

	// Query with a future time — should find nothing
	events, err = store.GetNewEvents("2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("got %d events, want 0", len(events))
	}
}

func TestGetNewChatMessages(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	msg := ChatMessage{
		ID:         200,
		ChatroomID: 50,
		SenderName: "Test User",
		Contents:   "Hello",
		SentAt:     "2026-03-02T10:00:00.000+01:00",
		RawJSON:    "{}",
	}
	if _, err := store.UpsertChatMessage(msg); err != nil {
		t.Fatal(err)
	}

	msgs, err := store.GetNewChatMessages("2000-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].SenderName != "Test User" {
		t.Fatalf("got sender %q", msgs[0].SenderName)
	}

	msgs, err = store.GetNewChatMessages("2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("got %d messages, want 0", len(msgs))
	}
}

func TestGetLatestAnnouncements(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Insert two announcements in group 1 (older + newer)
	store.UpsertEvent(Event{ID: 1, DType: "event.RAnnouncementEventPrimer", Title: "Old", Contents: "old body", SortDate: "2026-03-01T10:00:00+01:00", GroupID: 10, GroupName: "MB 2", AuthorName: "Jan Jansen", RawJSON: "{}"})
	store.UpsertEvent(Event{ID: 2, DType: "event.RAnnouncementEventPrimer", Title: "New", Contents: "new body", SortDate: "2026-03-02T10:00:00+01:00", GroupID: 10, GroupName: "MB 2", AuthorName: "Piet Puk", RawJSON: "{}"})

	// Insert one announcement in group 2
	store.UpsertEvent(Event{ID: 3, DType: "event.RAnnouncementEventPrimer", Title: "Group2", Contents: "g2 body", SortDate: "2026-03-01T12:00:00+01:00", GroupID: 20, GroupName: "Klas 3", AuthorName: "Marie", RawJSON: "{}"})

	// Insert a calendar event (should NOT appear)
	store.UpsertEvent(Event{ID: 4, DType: "event.RCalendarItemEventPrimer", Title: "Cal", SortDate: "2026-03-03T10:00:00+01:00", GroupID: 10, RawJSON: "{}"})

	got, err := store.GetLatestAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d announcements, want 2", len(got))
	}
	// Should be newest first
	if got[0].Title != "New" {
		t.Fatalf("first title = %q, want New", got[0].Title)
	}
	if got[0].GroupName != "MB 2" || got[0].AuthorName != "Piet Puk" {
		t.Fatalf("first metadata = group=%q author=%q", got[0].GroupName, got[0].AuthorName)
	}
	if got[1].Title != "Group2" {
		t.Fatalf("second title = %q, want Group2", got[1].Title)
	}
	if got[1].GroupName != "Klas 3" {
		t.Fatalf("second group = %q, want Klas 3", got[1].GroupName)
	}
}

func TestGetLatestChatMessagePerRoom(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Room 10: two messages, should only return the latest
	store.UpsertChatMessage(ChatMessage{ID: 1, ChatroomID: 10, ChatroomName: "Room Alpha", SenderName: "Alice", Contents: "old msg", SentAt: "2026-03-01T10:00:00+01:00", RawJSON: "{}"})
	store.UpsertChatMessage(ChatMessage{ID: 2, ChatroomID: 10, ChatroomName: "Room Alpha", SenderName: "Bob", Contents: "new msg", SentAt: "2026-03-02T10:00:00+01:00", RawJSON: "{}"})

	// Room 20: one message
	store.UpsertChatMessage(ChatMessage{ID: 3, ChatroomID: 20, ChatroomName: "Room Beta", SenderName: "Carol", Contents: "hello", SentAt: "2026-03-01T15:00:00+01:00", RawJSON: "{}"})

	got, err := store.GetLatestChatMessagePerRoom()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	// Should be newest first
	if got[0].Contents != "new msg" || got[0].SenderName != "Bob" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[0].ChatroomName != "Room Alpha" {
		t.Fatalf("first chatroom = %q, want Room Alpha", got[0].ChatroomName)
	}
	if got[1].Contents != "hello" || got[1].SenderName != "Carol" {
		t.Fatalf("second = %+v", got[1])
	}
	if got[1].ChatroomName != "Room Beta" {
		t.Fatalf("second chatroom = %q, want Room Beta", got[1].ChatroomName)
	}
}

func TestResetCache(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed data
	store.SetConfig("last_sync", "2026-03-02T00:00:00Z")
	store.UpsertEvent(Event{ID: 1, DType: "event.RAnnouncementEventPrimer", Title: "Ann", RawJSON: "{}"})
	store.UpsertChatMessage(ChatMessage{ID: 1, ChatroomID: 10, SenderName: "A", Contents: "hi", SentAt: "2026-03-02T10:00:00+01:00", RawJSON: "{}"})

	if err := store.ResetCache(); err != nil {
		t.Fatal(err)
	}

	// last_sync must be gone
	if _, err := store.GetConfig("last_sync"); err == nil {
		t.Fatal("last_sync should be deleted")
	}

	// Data tables must be empty
	events, _ := store.GetNewEvents("2000-01-01T00:00:00Z")
	if len(events) != 0 {
		t.Fatalf("got %d events, want 0", len(events))
	}
	msgs, _ := store.GetNewChatMessages("2000-01-01T00:00:00Z")
	if len(msgs) != 0 {
		t.Fatalf("got %d messages, want 0", len(msgs))
	}
}

func TestGetEventsSince(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.UpsertEvent(Event{ID: 1, DType: "event.RAnnouncementEventPrimer", Title: "Old", SortDate: "2026-03-01T10:00:00+01:00", RawJSON: "{}"})
	store.UpsertEvent(Event{ID: 2, DType: "event.RAnnouncementEventPrimer", Title: "New", SortDate: "2026-03-03T10:00:00+01:00", RawJSON: "{}"})

	// Should find only the newer event
	events, err := store.GetEventsSince("2026-03-02T00:00:00+01:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Title != "New" {
		t.Fatalf("got title %q, want New", events[0].Title)
	}

	// Should find both
	events, err = store.GetEventsSince("2026-03-01T00:00:00+01:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
}

func TestGetChatMessagesSince(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.UpsertChatMessage(ChatMessage{ID: 1, ChatroomID: 10, ChatroomName: "Room", SenderName: "Alice", Contents: "old", SentAt: "2026-03-01T10:00:00+01:00", RawJSON: "{}"})
	store.UpsertChatMessage(ChatMessage{ID: 2, ChatroomID: 10, ChatroomName: "Room", SenderName: "Bob", Contents: "new", SentAt: "2026-03-03T10:00:00+01:00", RawJSON: "{}"})

	msgs, err := store.GetChatMessagesSince("2026-03-02T00:00:00+01:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d msgs, want 1", len(msgs))
	}
	if msgs[0].SenderName != "Bob" {
		t.Fatalf("got sender %q, want Bob", msgs[0].SenderName)
	}
}

func TestGetLatestEmpty(t *testing.T) {
	store, err := Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	anns, err := store.GetLatestAnnouncements()
	if err != nil {
		t.Fatal(err)
	}
	if len(anns) != 0 {
		t.Fatalf("got %d, want 0", len(anns))
	}

	msgs, err := store.GetLatestChatMessagePerRoom()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("got %d, want 0", len(msgs))
	}
}
