package api

import "encoding/json"

// TokenResponse is the OAuth2 token refresh response from ParnaSys IDP.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ListResponse wraps the standard Parro API list response.
type ListResponse struct {
	Items []json.RawMessage `json:"items"`
}

// Link represents an API resource link.
type Link struct {
	ID   int64  `json:"id"`
	Rel  string `json:"rel"`
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
}

// Account represents the /account/me response.
type Account struct {
	DType       string `json:"dtype"`
	Links       []Link `json:"links"`
	AccountType string `json:"accountType"`
	Identity    struct {
		Links     []Link `json:"links"`
		Guardians []struct {
			Links []Link `json:"links"`
		} `json:"guardians"`
	} `json:"identity"`
}

// GuardianID extracts the guardian ID from the account's identity.guardians[0].
func (a *Account) GuardianID() int64 {
	if len(a.Identity.Guardians) > 0 {
		return SelfID(a.Identity.Guardians[0].Links)
	}
	return 0
}

// Group represents a school/class group.
type Group struct {
	DType       string `json:"dtype"`
	Links       []Link `json:"links"`
	Name        string `json:"name"`
	Schooljaar  int    `json:"schooljaar"`
	Type        string `json:"type"`
	UnreadCount int    `json:"unreadCount"`
}

// Announcement represents an announcement event.
type Announcement struct {
	DType       string         `json:"dtype"`
	Links       []Link         `json:"links"`
	Title       string         `json:"title"`
	Contents    string         `json:"contents"`
	SortDate    string         `json:"sortDate"`
	CreatedAt   string         `json:"createdAt"`
	Read        bool           `json:"read"`
	Deleted     bool           `json:"deleted"`
	Owner       IdentityPrimer `json:"owner"`
	Attachments []Attachment   `json:"attachments,omitempty"`
}

// CalendarEvent represents a calendar item event.
type CalendarEvent struct {
	DType     string `json:"dtype"`
	Links     []Link `json:"links"`
	Title     string `json:"title,omitempty"`
	SortDate  string `json:"sortDate"`
	CreatedAt string `json:"createdAt"`
	Deleted   bool   `json:"deleted"`
	Read      bool   `json:"read"`
	Cancelled bool   `json:"cancelled"`
	Children  []struct {
		DType string `json:"dtype"`
		Child struct {
			ID int64 `json:"id"`
		} `json:"child"`
	} `json:"children,omitempty"`
}

// CalendarEventDetail is the full calendar event (fetched per-item).
type CalendarEventDetail struct {
	DType        string `json:"dtype"`
	Links        []Link `json:"links"`
	SortDate     string `json:"sortDate"`
	Cancelled    bool   `json:"cancelled"`
	CalendarItem struct {
		Title     string `json:"title"`
		Type      string `json:"type"`
		StartDate string `json:"startDate"`
		EndDate   string `json:"endDate"`
	} `json:"calendarItem"`
	Children []struct {
		Child struct {
			Firstname string `json:"firstname"`
			Surname   string `json:"surname"`
		} `json:"child"`
	} `json:"children,omitempty"`
}

// ChatRoom represents a chatroom.
type ChatRoom struct {
	DType          string `json:"dtype"`
	Links          []Link `json:"links"`
	LastModifiedAt string `json:"lastModifiedAt"`
	SortDate       string `json:"sortDate"`
	Title          string `json:"title"`
}

// IdentityPrimer represents a nested identity with name info.
type IdentityPrimer struct {
	DType     string `json:"dtype"`
	Links     []Link `json:"links"`
	Firstname string `json:"firstname"`
	Surname   string `json:"surname"`
}

// AttachmentEntry is a single file within an attachment (SOURCE or THUMBNAIL).
type AttachmentEntry struct {
	DType       string `json:"dtype"`
	Type        string `json:"type"` // "SOURCE", "THUMBNAIL"
	URL         string `json:"url"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	Filename    string `json:"filename,omitempty"`
}

// Attachment represents a chat message attachment (image or video).
type Attachment struct {
	DType          string            `json:"dtype"`
	AttachmentType string            `json:"attachmentType"` // "IMAGE", "VIDEO"
	Entries        []AttachmentEntry `json:"entries"`
}

// SourceEntry returns the first entry with type "SOURCE", or nil.
func (a *Attachment) SourceEntry() *AttachmentEntry {
	for i := range a.Entries {
		if a.Entries[i].Type == "SOURCE" {
			return &a.Entries[i]
		}
	}
	return nil
}

// ChatMessage represents a chat message.
type ChatMessage struct {
	DType          string         `json:"dtype"`
	Links          []Link         `json:"links"`
	LastModifiedAt string         `json:"lastModifiedAt"`
	Identity       IdentityPrimer `json:"identity"`
	Text           string         `json:"text"`
	Attachment     *Attachment    `json:"attachment,omitempty"`
}

// SelfID extracts the numeric ID from the "self" link.
func SelfID(links []Link) int64 {
	for _, l := range links {
		if l.Rel == "self" {
			return l.ID
		}
	}
	return 0
}
