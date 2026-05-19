package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultTokenURL = "https://inloggen.parnassys.net/idp/oauth2/token?client_id=wfK7SFH8gojFJpREk5bx"
	DefaultBaseURL  = "https://rest-v2.parro.com/rest/v2"
	contentType     = "application/vnd.topicus.geon+json;version=216"
)

// Client is the Parro API client.
type Client struct {
	RefreshTokenValue string
	GuardianID        string
	accessToken       string

	// Overridable for testing
	TokenURL   string
	BaseURL    string
	HTTPClient *http.Client
	Logger     *log.Logger
}

func (c *Client) logf(format string, args ...any) {
	if c.Logger != nil {
		c.Logger.Printf(format, args...)
	}
}

// NewClient creates a new API client with defaults.
func NewClient(refreshToken, guardianID string) *Client {
	return &Client{
		RefreshTokenValue: refreshToken,
		GuardianID:        guardianID,
		TokenURL:          DefaultTokenURL,
		BaseURL:           DefaultBaseURL,
		HTTPClient:        http.DefaultClient,
	}
}

// SetAccessToken sets the access token directly (used after login).
func (c *Client) SetAccessToken(token string) {
	c.accessToken = token
}

// RefreshAccessToken exchanges the refresh token for a new access + refresh token pair.
func (c *Client) RefreshAccessToken() error {
	c.logf("refreshing access token via %s", c.TokenURL)
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshTokenValue},
	}
	start := time.Now()
	resp, err := c.HTTPClient.Post(c.TokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()
	c.logf("token endpoint responded %d (%s)", resp.StatusCode, time.Since(start))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh: status %d: %s", resp.StatusCode, body)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("token decode: %w", err)
	}

	c.accessToken = tok.AccessToken
	c.RefreshTokenValue = tok.RefreshToken
	c.logf("token refreshed successfully")
	return nil
}

// do performs an authenticated API request.
func (c *Client) do(method, path string, result any) error {
	reqURL := c.BaseURL + path
	req, err := http.NewRequest(method, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if c.GuardianID != "" {
		req.Header.Set("parro-authorization-role", "GUARDIAN:"+c.GuardianID)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentType)

	start := time.Now()
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("api %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	c.logf("%s %s → %d (%s)", method, path, resp.StatusCode, time.Since(start))

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api %s %s: status %d: %s", method, path, resp.StatusCode, body)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("api decode %s: %w", path, err)
		}
	}
	return nil
}

// getList fetches a list endpoint and unmarshals each item into T.
func getList[T any](c *Client, path string) ([]T, error) {
	var lr ListResponse
	if err := c.do(http.MethodGet, path, &lr); err != nil {
		return nil, err
	}
	items := make([]T, 0, len(lr.Items))
	for _, raw := range lr.Items {
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("unmarshal item: %w", err)
		}
		items = append(items, item)
	}
	c.logf("%s: %d items", path, len(items))
	return items, nil
}

// GetAccount fetches the current user's account.
func (c *Client) GetAccount() (*Account, error) {
	var acct Account
	if err := c.do(http.MethodGet, "/account/me", &acct); err != nil {
		return nil, err
	}
	return &acct, nil
}

// GetGroups fetches home groups (school + classes).
func (c *Client) GetGroups() ([]Group, error) {
	return getList[Group](c, "/group?dtype=identity.RHomeGroup")
}

// GetAnnouncements fetches announcements for a group.
func (c *Client) GetAnnouncements(groupID int64) ([]Announcement, error) {
	return getList[Announcement](c, fmt.Sprintf("/event?dtype=event.RAnnouncementEventPrimer&group=%d", groupID))
}

// GetCalendarEvents fetches calendar events since a given date (ISO8601).
func (c *Client) GetCalendarEvents(since string) ([]CalendarEvent, error) {
	return getList[CalendarEvent](c, "/event?dtype=event.RCalendarItemEventPrimer&sort=desc-stream&sortDateSince="+url.QueryEscape(since))
}

// GetCalendarEventDetail fetches the full detail for a calendar event.
func (c *Client) GetCalendarEventDetail(eventID int64) (*CalendarEventDetail, error) {
	var detail CalendarEventDetail
	if err := c.do(http.MethodGet, fmt.Sprintf("/event/%d?dtype=event.RCalendarItemEvent", eventID), &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// GetChatRooms fetches active chatrooms.
func (c *Client) GetChatRooms() ([]ChatRoom, error) {
	return getList[ChatRoom](c, "/chatroom")
}

// GetChatMessages fetches messages for a chatroom.
func (c *Client) GetChatMessages(chatroomID int64) ([]ChatMessage, error) {
	return getList[ChatMessage](c, fmt.Sprintf("/chatroom/%d/chatmessage", chatroomID))
}

// Get performs an authenticated GET and decodes the response into result.
func (c *Client) Get(path string, result any) error {
	return c.do(http.MethodGet, path, result)
}

// GetRawItems fetches a list endpoint and returns the raw JSON per item.
func (c *Client) GetRawItems(path string) ([]json.RawMessage, error) {
	var lr ListResponse
	if err := c.do(http.MethodGet, path, &lr); err != nil {
		return nil, err
	}
	c.logf("%s: %d raw items", path, len(lr.Items))
	return lr.Items, nil
}

// DownloadFile downloads a URL to destPath. It skips the download if destPath
// already exists with matching size. Returns true if the file was downloaded.
func (c *Client) DownloadFile(fileURL, destPath string, expectedSize int64) (downloaded bool, err error) {
	// Skip if already downloaded with correct size
	if info, err := os.Stat(destPath); err == nil && info.Size() == expectedSize {
		c.logf("skip download %s (already exists)", destPath)
		return false, nil
	}

	c.logf("downloading %s → %s", fileURL, destPath)
	start := time.Now()

	resp, err := c.HTTPClient.Get(fileURL)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("download %s: status %d", fileURL, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", destPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(destPath)
		return false, fmt.Errorf("write %s: %w", destPath, err)
	}

	c.logf("downloaded %s (%s)", destPath, time.Since(start))
	return true, nil
}
