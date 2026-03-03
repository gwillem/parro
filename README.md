# Parro CLI — Sync School Messages to Your Terminal

Parro CLI syncs announcements, calendar events, and chat messages from the [Parro](https://www.parro.com/) parent-school communication platform into a local SQLite database. It reverse-engineers the Parro REST API v2 so you can read school updates from your terminal, pipe them into scripts, or build your own integrations.

Parro (by Topicus/ParnaSys) is used by thousands of Dutch primary schools to communicate with parents. This tool gives you offline access to all your school messages without needing the mobile app.

Built as the data source for [OpenClaw](https://openclaw.org/) — an open platform that aggregates Dutch school communication (Parro, Social Schools, Schoudercom) into a single feed with push notifications, shared family calendars, and AI summaries.

## Features

- **Announcements** — school and class announcements with attachments
- **Calendar events** — upcoming school events (study days, holidays, parent nights)
- **Chat messages** — parent-teacher conversations with image/file downloads
- **Multi-account** — support for multiple children/schools
- **Incremental sync** — only fetches new items since last check
- **Local SQLite** — query your data with any SQL tool
- **Zero dependencies** — single binary, pure Go (no CGO)

## Install

```bash
go install github.com/gwillem/parro/cmd/parro@latest
```

Or build from source:

```bash
git clone https://github.com/gwillem/parro.git
cd parro
go test ./...
go install ./cmd/parro
```

## Usage

### Login

Authenticate with your ParnaSys credentials (the same email/password you use for the Parro app):

```bash
parro login user@example.com 'your-password'
```

This performs the full OAuth2+PKCE login flow and stores your refresh token in `~/.config/parro/config.json`.

### Sync

Fetch new messages:

```bash
parro check
```

First run seeds the local database silently. Subsequent runs print only new items since last sync:

```
=== New Announcements ===

[2026-03-03T09:15:00+01:00] [Groep 5] Weekly update (by Juf Anna)
Dear parents, this week we will be working on...

=== New Chat Messages ===

[2026-03-03T10:30:00+01:00] [Emma conversation] Juf Petra: See you at the parent meeting!
```

### Reset

Clear cached messages and re-sync from scratch (keeps your login):

```bash
parro reset
```

### Multiple accounts

If you have multiple children at different schools, log in with each account. Parro CLI stores separate databases per guardian ID:

```bash
parro login parent1@school-a.nl 'password1'
parro login parent2@school-b.nl 'password2'
```

When multiple accounts exist, specify which one to sync by username or guardian ID:

```bash
parro check -a parent1@school-a.nl
parro check -a 1818384531
```

With a single account, `-a` is not needed.

### Verbose output

```bash
parro check -v      # debug logging
parro check -vv     # debug + show latest messages per group/room
```

## Data storage

| What        | Path                                                                 |
| ----------- | -------------------------------------------------------------------- |
| Credentials | `$XDG_CONFIG_HOME/parro/config.json` (`~/.config/parro/config.json`) |
| Database    | `$XDG_DATA_HOME/parro/{guardian-id}.db` (`~/.local/share/parro/`)    |
| Attachments | `$XDG_CACHE_HOME/parro/{guardian-id}/` (`~/.cache/parro/`)           |

The SQLite database can be queried directly:

```bash
sqlite3 ~/.local/share/parro/1818384531.db "SELECT title, sort_date FROM events ORDER BY sort_date DESC LIMIT 5"
```

## OpenClaw integration

Parro CLI is designed as a data source for [OpenClaw](https://openclaw.org/). To set it up, log in once and then configure your OpenClaw bot to run `parro check` periodically:

```bash
# Initial setup
parro login user@example.com 'your-password'

# First run seeds the database (no output)
parro check

# Subsequent runs print only new items — feed this to your bot
parro check
```

The output of `parro check` contains new announcements, calendar events, and chat messages since the last sync. Point your OpenClaw bot at this command and instruct it to summarize important announcements and extract action items (permission slips, payment deadlines, volunteer requests, etc.).

Example cron entry to sync every 15 minutes:

```cron
*/15 * * * * parro check 2>/dev/null
```

## How it works

Parro CLI reverse-engineers the Parro REST API v2 (`rest-v2.parro.com`). The login flow replicates the mobile app's OAuth2+PKCE authentication against the ParnaSys identity provider. After login, it uses rolling refresh tokens to maintain access.

See [`doc/api.md`](doc/api.md) for the full reverse-engineered API specification.

## License

MIT
