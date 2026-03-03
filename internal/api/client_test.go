package api

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.FormValue("refresh_token"); got != "old-refresh" {
			t.Fatalf("refresh_token = %q", got)
		}
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
		})
	}))
	defer srv.Close()

	c := &Client{
		RefreshTokenValue: "old-refresh",
		GuardianID:        "123",
		TokenURL:          srv.URL,
		HTTPClient:        srv.Client(),
	}

	if err := c.RefreshAccessToken(); err != nil {
		t.Fatal(err)
	}
	if c.accessToken != "new-access" {
		t.Fatalf("accessToken = %q", c.accessToken)
	}
	if c.RefreshTokenValue != "new-refresh" {
		t.Fatalf("refreshToken = %q", c.RefreshTokenValue)
	}
}

func TestGetAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.URL.Path != "/rest/v2/account/me" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Account{
			DType:       "auth.RAccount",
			Links:       []Link{{ID: 1818364651, Rel: "self"}},
			AccountType: "GUARDIAN",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	acct, err := c.GetAccount()
	if err != nil {
		t.Fatal(err)
	}
	if acct.AccountType != "GUARDIAN" {
		t.Fatalf("accountType = %q", acct.AccountType)
	}
}

func TestGetGroups(t *testing.T) {
	groups := []Group{
		{DType: "identity.RHomeGroup", Links: []Link{{ID: 111, Rel: "self"}}, Name: "School", Type: "SCHOOLWIDE"},
		{DType: "identity.RHomeGroup", Links: []Link{{ID: 222, Rel: "self"}}, Name: "Klas 3", Type: "CLASS"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		items := make([]json.RawMessage, len(groups))
		for i, g := range groups {
			items[i], _ = json.Marshal(g)
		}
		json.NewEncoder(w).Encode(ListResponse{Items: items})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.GetGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d groups", len(got))
	}
	if got[0].Name != "School" {
		t.Fatalf("name = %q", got[0].Name)
	}
}

func TestGetAnnouncements(t *testing.T) {
	ann := Announcement{
		DType:    "event.RAnnouncementEventPrimer",
		Links:    []Link{{ID: 555, Rel: "self"}},
		Title:    "Test Ann",
		Contents: "Hello",
		SortDate: "2026-03-02T21:18:27.383+01:00",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.URL.Query().Get("group") != "111" {
			t.Fatalf("group param = %q", r.URL.Query().Get("group"))
		}
		raw, _ := json.Marshal(ann)
		json.NewEncoder(w).Encode(ListResponse{Items: []json.RawMessage{raw}})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.GetAnnouncements(111)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Test Ann" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGetChatRooms(t *testing.T) {
	room := ChatRoom{
		DType: "chat.RChatRoomPrimer",
		Links: []Link{{ID: 999, Rel: "self"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		raw, _ := json.Marshal(room)
		json.NewEncoder(w).Encode(ListResponse{Items: []json.RawMessage{raw}})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.GetChatRooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || SelfID(got[0].Links) != 999 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGetChatMessages(t *testing.T) {
	msg := ChatMessage{
		DType: "chat.RChatTextMessage",
		Links: []Link{{ID: 777, Rel: "self"}},
		Identity: IdentityPrimer{
			Firstname: "Anna",
			Surname:   "Bakker",
		},
		Text: "Hallo!",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertHeaders(t, r)
		if r.URL.Path != "/rest/v2/chatroom/999/chatmessage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		raw, _ := json.Marshal(msg)
		json.NewEncoder(w).Encode(ListResponse{Items: []json.RawMessage{raw}})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.GetChatMessages(999)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "Hallo!" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got[0].Identity.Firstname != "Anna" {
		t.Fatalf("sender = %q", got[0].Identity.Firstname)
	}
}

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		GuardianID:  "123",
		accessToken: "test-token",
		BaseURL:     srv.URL + "/rest/v2",
		HTTPClient:  srv.Client(),
	}
}

func assertHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := r.Header.Get("parro-authorization-role"); got != "GUARDIAN:123" {
		t.Fatalf("parro-authorization-role = %q", got)
	}
	ct := "application/vnd.topicus.geon+json;version=216"
	if got := r.Header.Get("Content-Type"); got != ct {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := r.Header.Get("Accept"); got != ct {
		t.Fatalf("Accept = %q", got)
	}
}

func TestVerboseLogging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Account{DType: "auth.RAccount"})
	}))
	defer srv.Close()

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	c := &Client{
		GuardianID:  "123",
		accessToken: "test-token",
		BaseURL:     srv.URL + "/rest/v2",
		HTTPClient:  srv.Client(),
		Logger:      logger,
	}

	if _, err := c.GetAccount(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "GET") {
		t.Fatalf("expected log to contain HTTP method, got: %q", output)
	}
	if !strings.Contains(output, "/account/me") {
		t.Fatalf("expected log to contain path, got: %q", output)
	}
	if !strings.Contains(output, "200") {
		t.Fatalf("expected log to contain status code, got: %q", output)
	}
}

func TestDownloadFile(t *testing.T) {
	content := []byte("fake image data here")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	c := &Client{HTTPClient: srv.Client()}
	dest := filepath.Join(t.TempDir(), "test.jpg")

	// First download
	downloaded, err := c.DownloadFile(srv.URL+"/image.jpg", dest, int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if !downloaded {
		t.Fatal("expected downloaded=true on first call")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content = %q", got)
	}

	// Second download should skip (file exists with matching size)
	downloaded, err = c.DownloadFile(srv.URL+"/image.jpg", dest, int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if downloaded {
		t.Fatal("expected downloaded=false on second call (already exists)")
	}
}

func TestDownloadFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{HTTPClient: srv.Client()}
	dest := filepath.Join(t.TempDir(), "missing.jpg")

	_, err := c.DownloadFile(srv.URL+"/missing", dest, 100)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestSourceEntry(t *testing.T) {
	a := &Attachment{
		Entries: []AttachmentEntry{
			{Type: "THUMBNAIL", URL: "https://example.com/thumb.jpg"},
			{Type: "SOURCE", URL: "https://example.com/full.jpg", Size: 1234},
		},
	}
	src := a.SourceEntry()
	if src == nil {
		t.Fatal("expected non-nil source entry")
	}
	if src.URL != "https://example.com/full.jpg" {
		t.Fatalf("url = %q", src.URL)
	}

	// No source entry
	a2 := &Attachment{Entries: []AttachmentEntry{{Type: "THUMBNAIL"}}}
	if a2.SourceEntry() != nil {
		t.Fatal("expected nil for no source entry")
	}
}

func TestChatMessageWithAttachment(t *testing.T) {
	msg := ChatMessage{
		DType: "chat.RChatTextMessage",
		Links: []Link{{ID: 777, Rel: "self"}},
		Text:  "",
		Attachment: &Attachment{
			AttachmentType: "IMAGE",
			Entries: []AttachmentEntry{
				{Type: "SOURCE", URL: "https://cdn.example.com/img.jpg", Size: 5000, ContentType: "image/jpeg"},
			},
		},
	}

	// Verify round-trip JSON marshaling preserves attachment
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ChatMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Attachment == nil {
		t.Fatal("attachment lost in round-trip")
	}
	src := decoded.Attachment.SourceEntry()
	if src == nil || src.URL != "https://cdn.example.com/img.jpg" {
		t.Fatalf("source entry = %+v", src)
	}
}

func TestNilLoggerNoPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Account{DType: "auth.RAccount"})
	}))
	defer srv.Close()

	c := &Client{
		GuardianID:  "123",
		accessToken: "test-token",
		BaseURL:     srv.URL + "/rest/v2",
		HTTPClient:  srv.Client(),
		// Logger intentionally nil
	}

	if _, err := c.GetAccount(); err != nil {
		t.Fatal(err)
	}
}
