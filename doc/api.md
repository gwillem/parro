# Parro REST API v2

Reverse-engineered specification of the Parro parent-school communication platform API.

## Base URLs

| Service | URL |
|---------|-----|
| IDP (OAuth2) | `https://inloggen.parnassys.net/idp/oauth2` |
| REST API v2 | `https://rest-v2.parro.com/rest/v2` |

## Authentication

### OAuth2 + PKCE login flow

The login is a multi-step browser-like OAuth2 flow using PKCE (S256).

**Client ID:** `wfK7SFH8gojFJpREk5bx`
**Redirect URI:** `parro://oauth2`
**User-Agent:** `parro/2 CFNetwork/3860.300.31 Darwin/25.2.0`

1. **GET** `/idp/oauth2/authorize?client_id={id}&redirect_uri=parro://oauth2&response_type=code&scope=openid&oauth2=authorize&state={state}&code_challenge={challenge}&code_challenge_method=S256`
   - Redirects (302) to the login page at `/idp/?auth={jwt}`

2. **GET** login page URL (follow redirect from step 1)
   - Returns HTML with a `<form>` containing the login action URL

3. **POST** form action URL extracted from HTML
   - Content-Type: `application/x-www-form-urlencoded`
   - Body: `aanmelden=x&e-mailadres={email}&wachtwoord={password}`
   - Success: 302 redirect to `/idp/wicket/page?{N}`
   - Failure: 200 with Wicket feedback panel error in HTML

4. **GET** Wicket account selection page (follow redirect from step 3)
   - Loads the account selection page (required before AJAX call)

5. **GET** `{pageURL}-1.0-accountKeuze-accounts-account-1&_={timestamp}`
   - Wicket AJAX call to select the first account
   - Accept: `application/xml, text/xml, */*; q=0.01`
   - Returns AJAX XML with `<redirect>` containing `parro://oauth2:443/?code={code}`
   - Or returns 302 with auth code in Location header

6. **POST** `/idp/oauth2/token?client_id={id}&code={code}&grant_type=authorization_code&code_verifier={verifier}`
   - Content-Type: `application/x-www-form-urlencoded`
   - Returns: `TokenResponse`

### Token refresh

**POST** `https://inloggen.parnassys.net/idp/oauth2/token?client_id=wfK7SFH8gojFJpREk5bx`
- Content-Type: `application/x-www-form-urlencoded`
- Body: `grant_type=refresh_token&refresh_token={token}`
- Returns new access + refresh token pair (rolling refresh tokens)

```json
{
  "access_token": "...",
  "refresh_token": "..."
}
```

## API conventions

### Request headers

All REST API requests require:

| Header | Value |
|--------|-------|
| `Authorization` | `Bearer {access_token}` |
| `Content-Type` | `application/vnd.topicus.geon+json;version=216` |
| `Accept` | `application/vnd.topicus.geon+json;version=216` |
| `parro-authorization-role` | `GUARDIAN:{guardian_id}` |

### List responses

All list endpoints return a wrapper object:

```json
{
  "items": [ ... ]
}
```

### Links

Every resource has a `links` array. The `self` link contains the resource ID:

```json
{
  "links": [
    { "id": 1234567890, "rel": "self", "type": "..." }
  ]
}
```

Resources may also have a `koppeling` link (used in chatrooms for member associations).

### DTypes

Every resource has a `dtype` field indicating the concrete type (Java-style):

| dtype | Description |
|-------|-------------|
| `auth.RAccount` | User account |
| `identity.RIdentityPrimer` | Identity (person) primer |
| `identity.RHomeGroup` | School/class group |
| `event.RAnnouncementEventPrimer` | Announcement |
| `event.RCalendarItemEventPrimer` | Calendar event |
| `chat.RChatRoomPrimer` | Chatroom |
| `chat.RChatTextMessage` | Chat text message |

### Common fields

Most resources include these fields (omitted from examples below for brevity):

- `permissions` — array of permission objects
- `additionalObjects` — usually `{}`
- `lastModifiedAt` — ISO8601 timestamp

## Endpoints

### GET /account/me

Returns the authenticated user's account.

```json
{
  "dtype": "auth.RAccount",
  "links": [{ "id": 1000000001, "rel": "self", "type": "auth.RAccount" }],
  "accountType": "GUARDIAN",
  "username": "12345678900",
  "email": "user@example.com",
  "organisation": {
    "dtype": "auth.ROrganisationPrimer",
    "name": "Example School",
    "code": "01AB01",
    "active": true
  },
  "identity": {
    "dtype": "identity.RIdentity",
    "links": [{ "id": 1000000002, "rel": "self" }],
    "firstname": "Jan",
    "surname": "de Vries",
    "role": "GUARDIAN",
    "guardians": [
      {
        "dtype": "identity.RGuardianPrimer",
        "links": [{ "id": 1000000003, "rel": "self" }],
        "firstname": "Jan",
        "surname": "de Vries",
        "role": "GUARDIAN",
        "childNames": ["Emma", "Lucas"]
      }
    ]
  }
}
```

The guardian ID is extracted from `identity.guardians[0].links` (self link).

### GET /group?dtype=identity.RHomeGroup

Returns groups (schools + classes) the guardian belongs to.

```json
{
  "dtype": "identity.RHomeGroup",
  "links": [{ "id": 5000000001, "rel": "self", "type": "identity.RHomeGroup" }],
  "name": "Example School",
  "schooljaar": 2025,
  "stamgroep": false,
  "type": "SCHOOLWIDE",
  "unreadCount": 0,
  "memberMuted": false,
  "numberOfChildren": 0,
  "numberOfGuardians": 200,
  "numberOfTeachers": 30,
  "childAvatars": []
}
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Group display name |
| `schooljaar` | int | School year |
| `type` | string | `"SCHOOLWIDE"` or `"CLASS"` |
| `stamgroep` | bool | Whether this is a "stamgroep" (homeroom) |
| `unreadCount` | int | Number of unread items |
| `numberOfChildren` | int | Child count (0 for schoolwide) |
| `numberOfGuardians` | int | Guardian count |
| `numberOfTeachers` | int | Teacher count |

### GET /event?dtype=event.RAnnouncementEventPrimer&group={groupID}

Returns announcements for a group. Results are newest first.

```json
{
  "dtype": "event.RAnnouncementEventPrimer",
  "links": [{ "id": 5000000010, "rel": "self", "type": "event.RAnnouncementEventPrimer" }],
  "title": "Example announcement",
  "contents": "Dear parents, ...",
  "sortDate": "2026-03-02T17:11:10.852+01:00",
  "createdAt": "2026-03-02T17:11:10.853+01:00",
  "read": true,
  "deleted": false,
  "archived": false,
  "cancelled": false,
  "liked": false,
  "likeable": true,
  "draft": false,
  "pinned": false,
  "realisationReason": "RECIPIENT",
  "readCount": { "dtype": "event.REventReadCount", "liked": 0, "read": 0, "total": 0 },
  "owner": {
    "dtype": "identity.RIdentityPrimer",
    "links": [{ "id": 4000000001, "rel": "self" }],
    "firstname": "Maria",
    "surname": "Jansen",
    "role": "TEACHER"
  },
  "lastEditedBy": {
    "dtype": "identity.RIdentityPrimer",
    "firstname": "Maria",
    "surname": "Jansen",
    "role": "TEACHER"
  },
  "attachments": []
}
```

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Announcement title |
| `contents` | string | Announcement body text |
| `sortDate` | string | ISO8601 timestamp for ordering |
| `createdAt` | string | ISO8601 creation timestamp |
| `read` | bool | Whether the current user has read it |
| `deleted` | bool | Soft-deleted flag |
| `likeable` | bool | Whether likes are enabled |
| `owner` | object | Author identity (firstname, surname, role) |
| `lastEditedBy` | object | Last editor identity |
| `attachments` | array | Attached files/images |

### GET /event?dtype=event.RCalendarItemEventPrimer&sort=desc-stream&sortDateSince={since}

Returns calendar events since a given date.

```json
{
  "dtype": "event.RCalendarItemEventPrimer",
  "links": [{ "id": 5000000020, "rel": "self" }],
  "title": "Study day",
  "sortDate": "2026-03-10T08:00:00.000+01:00",
  "createdAt": "2026-02-15T12:00:00.000+01:00",
  "deleted": false,
  "read": false,
  "cancelled": false,
  "children": [
    {
      "dtype": "...",
      "child": { "id": 12345 }
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Event title (may be empty) |
| `sortDate` | string | Event date/time |
| `cancelled` | bool | Whether the event is cancelled |
| `children` | array | Associated children (optional) |

### GET /chatroom

Returns active chatrooms. Includes embedded `lastMessage`.

```json
{
  "dtype": "chat.RChatRoomPrimer",
  "links": [
    { "id": 5000000030, "rel": "koppeling", "type": "chat.RChatRoomPrimer" },
    { "id": 5000000031, "rel": "self", "type": "chat.RChatRoomPrimer" }
  ],
  "lastModifiedAt": "2026-03-02T22:48:43.818+01:00",
  "sortDate": "2026-03-02T22:48:43.821+01:00",
  "title": "Child conversation Emma de Vries",
  "type": "FAMILY",
  "active": true,
  "archived": false,
  "muted": false,
  "todo": false,
  "unreadCount": 0,
  "admin": false,
  "numberOfMembers": 5,
  "hasGuardians": true,
  "memberNames": ["Anna", "Jan", "Piet"],
  "childNames": ["Emma"],
  "repliesEnabled": true,
  "lastMessage": {
    "dtype": "chat.RChatTextMessage",
    "links": [{ "id": 5000000040, "rel": "self" }],
    "lastModifiedAt": "2026-03-02T22:48:43.818+01:00",
    "createdAt": "2026-03-02T22:48:43.818+01:00",
    "identity": {
      "dtype": "identity.RIdentityPrimer",
      "firstname": "Anna",
      "surname": "Bakker",
      "role": "TEACHER"
    },
    "text": "That sounds good!",
    "deleted": false,
    "allRead": false
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Chatroom display name |
| `type` | string | `"FAMILY"`, etc. |
| `active` | bool | Whether the room is active |
| `memberNames` | []string | First names of members |
| `childNames` | []string | Names of associated children |
| `lastMessage` | object | Most recent message (embedded) |
| `unreadCount` | int | Number of unread messages |
| `repliesEnabled` | bool | Whether replies are allowed |

### GET /chatroom/{chatroomID}/chatmessage

Returns messages for a chatroom.

```json
{
  "dtype": "chat.RChatTextMessage",
  "links": [{ "id": 5000000050, "rel": "self", "type": "chat.RChatTextMessage" }],
  "lastModifiedAt": "2026-03-02T22:48:43.818+01:00",
  "createdAt": "2026-03-02T22:48:43.818+01:00",
  "identity": {
    "dtype": "identity.RIdentityPrimer",
    "links": [{ "id": 4000000002, "rel": "self" }],
    "firstname": "Anna",
    "surname": "Bakker",
    "role": "TEACHER"
  },
  "text": "That sounds good!",
  "deleted": false,
  "allRead": false
}
```

| Field | Type | Description |
|-------|------|-------------|
| `text` | string | Message text (may be empty for non-text messages) |
| `identity` | object | Sender identity (firstname, surname, role) |
| `createdAt` | string | Message creation timestamp |
| `deleted` | bool | Whether the message was deleted |
| `allRead` | bool | Whether all recipients have read it |

## Notes

- Refresh tokens are rolling: each refresh returns a new refresh token, invalidating the previous one.
- The content type version (`version=216`) may need updating as the API evolves.
- List endpoints return items newest-first by default.
- The API uses "Primer" types (e.g., `RChatRoomPrimer`) which are lightweight representations; full types may exist with additional fields.
- Chat messages with empty `text` may represent attachments or images.
- The `identity` role can be `"TEACHER"` or `"GUARDIAN"`.
