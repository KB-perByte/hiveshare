# HiveShare API Reference

> **Verification:** All curl examples in this document are tested by `scripts/test-api-examples.sh`. If you add or modify an endpoint or example, update the script to match and run it to verify:
> ```bash
> ./scripts/test-api-examples.sh
> ```

## Table of Contents

- [Overview](#overview)
- [Health](#health)
  - [GET /health](#get-health)
- [Auth](#auth)
  - [POST /api/v1/auth/register](#post-register)
  - [GET /api/v1/auth/whoami](#get-whoami)
- [Hiveshares](#hiveshares)
  - [POST /api/v1/hiveshares](#post-create-hiveshare)
  - [GET /api/v1/hiveshares](#get-list-hiveshares)
  - [GET /api/v1/hiveshares/{id}](#get-hiveshare)
  - [PUT /api/v1/hiveshares/{id}](#put-update-hiveshare)
  - [DELETE /api/v1/hiveshares/{id}](#delete-hiveshare)
  - [POST /api/v1/hiveshares/{id}/invite](#post-invite)
  - [POST /api/v1/invitations/{token}/accept](#post-accept-invite)
  - [GET /api/v1/hiveshares/{id}/members](#get-members)
  - [DELETE /api/v1/hiveshares/{id}/members/{userId}](#delete-member)
- [Memory](#memory)
  - [POST /api/v1/hiveshares/{id}/memory](#post-create-entry)
  - [GET /api/v1/hiveshares/{id}/memory](#get-list-entries)
  - [GET /api/v1/hiveshares/{id}/memory/{entryId}](#get-entry)
  - [PUT /api/v1/hiveshares/{id}/memory/{entryId}](#put-update-entry)
  - [DELETE /api/v1/hiveshares/{id}/memory/{entryId}](#delete-entry)
  - [POST /api/v1/hiveshares/{id}/memory/search](#post-search)
  - [POST /api/v1/hiveshares/{id}/memory/copy](#post-copy-entries)
- [History](#history)
  - [GET /api/v1/hiveshares/{id}/memory/{entryId}/history](#get-history)
  - [POST /api/v1/hiveshares/{id}/memory/{entryId}/rollback](#post-rollback)
  - [POST /api/v1/hiveshares/{id}/memory/undelete](#post-undelete)
- [Snapshots](#snapshots)
  - [POST /api/v1/hiveshares/{id}/snapshots](#post-create-snapshot)
  - [GET /api/v1/hiveshares/{id}/snapshots](#get-list-snapshots)
  - [GET /api/v1/hiveshares/{id}/snapshots/{snapshotId}](#get-snapshot)
  - [POST /api/v1/hiveshares/{id}/snapshots/{snapshotId}/restore](#post-restore-snapshot)
  - [DELETE /api/v1/hiveshares/{id}/snapshots/{snapshotId}](#delete-snapshot)
- [Metrics](#metrics)
  - [GET /api/v1/hiveshares/{id}/metrics](#get-hiveshare-metrics)
  - [GET /api/v1/metrics/me](#get-user-metrics)
- [SSE Stream](#sse-stream)
  - [GET /api/v1/hiveshares/{id}/stream](#get-stream)

---

## Overview

Base URL: `http://localhost:8080` (configurable via `BASE_URL` env var)

**Authentication:** All endpoints except `/health`, `/api/v1/auth/register`, and `/api/v1/invitations/{token}/accept` require a Bearer token in the `Authorization` header:

```
Authorization: Bearer hvs_<token>
```

**Rate limiting:** 60 requests per minute per API key (or per IP if unauthenticated). Returns `429` when exceeded.

**Body size limit:** 1 MB max request body.

**Error format:** All errors return JSON:
```json
{"error": "description of the problem"}
```

---

## Health

### GET /health

`GET /health`

**Auth:** None

**Response:** `200 OK`
```json
{
  "status": "ok",
  "db": "ok",
  "redis": "ok",
  "commit": "d61ca5d",
  "build_time": "2026-07-22T20:13:28Z"
}
```

Returns `503` with `"status": "degraded"` if Postgres or Redis is unreachable.

**Example:**
```bash
curl http://localhost:8080/health
```

---

## Auth

### POST Register

`POST /api/v1/auth/register`

**Auth:** None

**Request body:**
```json
{
  "email": "user@example.com",
  "name": "Display Name"
}
```

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "name": "Display Name",
  "api_key": "hvs_<48 hex chars>",
  "created_at": "2026-07-22T10:00:00Z"
}
```

The `api_key` is returned only once. It is stored as a SHA-256 hash and cannot be retrieved again.

**Error responses:**
- `400` — email or name missing
- `409` — email already registered

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","name":"Alice"}'
```

---

### GET Whoami

`GET /api/v1/auth/whoami`

**Auth:** Required

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "name": "Display Name",
  "created_at": "2026-07-22T10:00:00Z"
}
```

**Error responses:**
- `401` — missing or invalid API key

**Example:**
```bash
curl http://localhost:8080/api/v1/auth/whoami \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

## Hiveshares

### POST Create Hiveshare

`POST /api/v1/hiveshares`

**Auth:** Required

**Request body:**
```json
{
  "name": "Sprint 42",
  "description": "Shared context for sprint 42"
}
```

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "name": "Sprint 42",
  "description": "Shared context for sprint 42",
  "owner_id": "uuid",
  "settings": {},
  "created_at": "2026-07-22T10:00:00Z",
  "updated_at": "2026-07-22T10:00:00Z",
  "role": "all",
  "member_count": 1
}
```

**Error responses:**
- `400` — name missing

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Sprint 42","description":"Shared context"}'
```

---

### GET List Hiveshares

`GET /api/v1/hiveshares`

**Auth:** Required

Returns all hiveshares the authenticated user is a member of.

**Response:** `200 OK`
```json
[
  {
    "id": "uuid",
    "name": "Sprint 42",
    "description": "...",
    "owner_id": "uuid",
    "role": "all",
    "member_count": 3,
    "created_at": "2026-07-22T10:00:00Z",
    "updated_at": "2026-07-22T10:00:00Z"
  }
]
```

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### GET Hiveshare

`GET /api/v1/hiveshares/{id}`

**Auth:** Required
**Access:** Must be a member

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "name": "Sprint 42",
  "description": "...",
  "owner_id": "uuid",
  "settings": {},
  "role": "all",
  "member_count": 3,
  "created_at": "2026-07-22T10:00:00Z",
  "updated_at": "2026-07-22T10:00:00Z"
}
```

**Error responses:**
- `404` — hiveshare not found or user is not a member

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### PUT Update Hiveshare

`PUT /api/v1/hiveshares/{id}`

**Auth:** Required
**Access:** CanWrite (role `all`)

**Request body:**
```json
{
  "name": "New Name",
  "description": "Updated description"
}
```

**Response:** `200 OK` (returns the updated hiveshare)

**Error responses:**
- `403` — view-only access

**Example:**
```bash
curl -X PUT http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Renamed","description":"Updated"}'
```

---

### DELETE Hiveshare

`DELETE /api/v1/hiveshares/{id}`

**Auth:** Required
**Access:** Owner only (`owner_id` must match)

**Response:** `204 No Content`

**Error responses:**
- `403` — not the owner
- `404` — hiveshare not found

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### POST Invite

`POST /api/v1/hiveshares/{id}/invite`

**Auth:** Required
**Access:** CanWrite (role `all`)

**Request body:**
```json
{
  "email": "bob@example.com",
  "role": "view"
}
```

`role` is `all` (read/write/invite) or `view` (read-only). Defaults to `all`.

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "hiveshare_id": "uuid",
  "email": "bob@example.com",
  "invited_by": "uuid",
  "token": "48-hex-chars",
  "role": "view",
  "status": "pending",
  "created_at": "2026-07-22T10:00:00Z",
  "expires_at": "2026-07-29T10:00:00Z",
  "invite_url": "http://localhost:8080/api/v1/invitations/TOKEN/accept"
}
```

Invitations expire after 7 days.

**Error responses:**
- `400` — email missing
- `403` — view-only access

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/invite \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"email":"bob@example.com","role":"view"}'
```

---

### POST Accept Invite

`POST /api/v1/invitations/{token}/accept`

**Auth:** None

**Request body (optional):**
```json
{
  "name": "Bob"
}
```

If `name` is omitted, the invited email is used as the display name. If the user does not exist, one is created.

**Response:** `200 OK`
```json
{
  "message": "Welcome to Sprint 42",
  "hiveshare_id": "uuid",
  "user": {
    "id": "uuid",
    "email": "bob@example.com",
    "name": "Bob",
    "api_key": "hvs_...",
    "created_at": "2026-07-22T10:00:00Z"
  }
}
```

**Error responses:**
- `404` — invitation not found
- `410` — invitation expired or already accepted

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/invitations/TOKEN/accept \
  -H "Content-Type: application/json" \
  -d '{"name":"Bob"}'
```

---

### GET Members

`GET /api/v1/hiveshares/{id}/members`

**Auth:** Required
**Access:** CanView

**Response:** `200 OK`
```json
[
  {
    "hiveshare_id": "uuid",
    "user_id": "uuid",
    "name": "Alice",
    "email": "alice@example.com",
    "role": "all",
    "joined_at": "2026-07-22T10:00:00Z"
  }
]
```

**Error responses:**
- `403` — not a member

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/members \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### DELETE Member

`DELETE /api/v1/hiveshares/{id}/members/{userId}`

**Auth:** Required
**Access:** CanWrite to remove others; any member can remove themselves

Cannot remove the owner (`owner_id`).

**Response:** `204 No Content`

**Error responses:**
- `403` — view-only trying to remove someone else

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/members/USER_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

## Memory

### POST Create Entry

`POST /api/v1/hiveshares/{id}/memory`

**Auth:** Required
**Access:** CanWrite

**Request body:**
```json
{
  "source_type": "jira",
  "source_ref": "PROJ-123",
  "source_url": "https://issues.example.com/PROJ-123",
  "tool": "claude",
  "content": "Analysis of the auth refactor...",
  "summary": "Auth refactor analysis",
  "tags": ["auth", "refactor"],
  "metadata": {"sprint": 42}
}
```

| Field | Required | Values |
|-------|----------|--------|
| `source_type` | Yes | `jira`, `github_issue`, `github_pr`, `file`, `url`, `manual` |
| `source_ref` | Yes | Free text (e.g. ticket ID, file path) |
| `content` | Yes | The memory content |
| `tool` | No | `claude`, `cursor`, `manual` (default: `manual`) |
| `source_url` | No | URL to the source |
| `summary` | No | Short summary |
| `tags` | No | Array of strings |
| `metadata` | No | Arbitrary JSON object |

Embedding is generated asynchronously after creation.

**Response:** `201 Created`
```json
{
  "id": "uuid",
  "hiveshare_id": "uuid",
  "user_id": "uuid",
  "user_name": "Alice",
  "source_type": "jira",
  "source_ref": "PROJ-123",
  "source_url": "https://issues.example.com/PROJ-123",
  "tool": "claude",
  "content": "Analysis of the auth refactor...",
  "summary": "Auth refactor analysis",
  "tags": ["auth", "refactor"],
  "metadata": {"sprint": 42},
  "views": 0,
  "reuses": 0,
  "created_at": "2026-07-22T10:00:00Z",
  "updated_at": "2026-07-22T10:00:00Z"
}
```

**Error responses:**
- `400` — content, source_type, or source_ref missing
- `403` — view-only access

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"source_type":"jira","source_ref":"PROJ-123","content":"Analysis...","tool":"claude","tags":["auth"]}'
```

---

### GET List Entries

`GET /api/v1/hiveshares/{id}/memory`

**Auth:** Required
**Access:** CanView

**Query parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 50 | Max entries to return |
| `offset` | 0 | Pagination offset |
| `source_type` | | Filter by source type |
| `source_ref` | | Filter by source reference |
| `tag` | | Filter by tag |
| `tool` | | Filter by tool |

List responses omit `content` to keep payloads small. Use the GET single entry endpoint for full content.

**Response:** `200 OK`
```json
[
  {
    "id": "uuid",
    "hiveshare_id": "uuid",
    "user_id": "uuid",
    "user_name": "Alice",
    "source_type": "jira",
    "source_ref": "PROJ-123",
    "summary": "Auth refactor analysis",
    "tags": ["auth"],
    "views": 5,
    "reuses": 2,
    "created_at": "2026-07-22T10:00:00Z"
  }
]
```

**Example:**
```bash
curl "http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory?source_type=jira&limit=10" \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### GET Entry

`GET /api/v1/hiveshares/{id}/memory/{entryId}`

**Auth:** Required
**Access:** CanView

Returns the full entry including content. Increments view counter.

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "hiveshare_id": "uuid",
  "user_id": "uuid",
  "user_name": "Alice",
  "source_type": "jira",
  "source_ref": "PROJ-123",
  "source_url": "https://...",
  "tool": "claude",
  "content": "Full content text...",
  "summary": "Auth refactor analysis",
  "tags": ["auth"],
  "metadata": {"sprint": 42},
  "views": 6,
  "reuses": 2,
  "created_at": "2026-07-22T10:00:00Z",
  "updated_at": "2026-07-22T10:00:00Z"
}
```

**Error responses:**
- `404` — entry not found

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/ENTRY_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### PUT Update Entry

`PUT /api/v1/hiveshares/{id}/memory/{entryId}`

**Auth:** Required
**Access:** CanWrite

**Request body:**
```json
{
  "content": "Updated analysis...",
  "summary": "Updated summary",
  "tags": ["auth", "updated"]
}
```

All fields are optional. Updating content triggers re-embedding.

**Response:** `200 OK` (returns the updated entry)

**Error responses:**
- `403` — view-only access

**Example:**
```bash
curl -X PUT http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/ENTRY_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"content":"Updated analysis...","tags":["auth","updated"]}'
```

---

### DELETE Entry

`DELETE /api/v1/hiveshares/{id}/memory/{entryId}`

**Auth:** Required
**Access:** CanWrite

**Response:** `204 No Content`

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/ENTRY_ID \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### POST Search

`POST /api/v1/hiveshares/{id}/memory/search`

**Auth:** Required
**Access:** CanView

Searches by semantic similarity (vector search) if embeddings are enabled, falls back to PostgreSQL full-text search otherwise.

**Request body:**
```json
{
  "query": "auth refactor approach",
  "source_type": "jira",
  "limit": 10
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `query` | Yes | | Search query text |
| `source_type` | No | | Filter results by source type |
| `limit` | No | 10 | Max results |

**Response:** `200 OK`
```json
{
  "results": [
    {
      "id": "uuid",
      "hiveshare_id": "uuid",
      "user_id": "uuid",
      "user_name": "Alice",
      "source_type": "jira",
      "source_ref": "PROJ-123",
      "content": "Full content...",
      "summary": "...",
      "tags": ["auth"],
      "views": 5,
      "reuses": 2,
      "score": 0.87,
      "created_at": "2026-07-22T10:00:00Z"
    }
  ],
  "count": 1,
  "query": "auth refactor approach"
}
```

**Error responses:**
- `400` — query missing

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/search \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"query":"auth refactor","limit":5}'
```

---

### POST Copy Entries

`POST /api/v1/hiveshares/{id}/memory/copy`

**Auth:** Required
**Access:** CanWrite on target hiveshare; CanView on source hiveshare(s)

Copies memory entries (including embeddings) from any accessible hiveshare into the target. Used for rollforward merges after a snapshot restore.

**Request body:**
```json
{
  "entry_ids": ["uuid-1", "uuid-2"]
}
```

**Response:** `201 Created`
```json
[
  {
    "id": "new-uuid",
    "hiveshare_id": "target-hiveshare-uuid",
    "content": "Copied content...",
    "source_type": "jira",
    "source_ref": "PROJ-123",
    ...
  }
]
```

Entries with NULL embeddings are queued for re-embedding.

**Error responses:**
- `400` — entry_ids missing or empty

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/TARGET_ID/memory/copy \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"entry_ids":["ENTRY_UUID_1","ENTRY_UUID_2"]}'
```

---

## History

### GET History

`GET /api/v1/hiveshares/{id}/memory/{entryId}/history`

**Auth:** Required
**Access:** CanView

Returns version history for a memory entry, including deleted entries.

**Query parameters:**

| Param | Default | Description |
|-------|---------|-------------|
| `limit` | 20 | Max versions |
| `offset` | 0 | Pagination offset |

**Response:** `200 OK`
```json
[
  {
    "history_id": 42,
    "entry_id": "uuid",
    "hiveshare_id": "uuid",
    "user_id": "uuid",
    "action": "update",
    "content": "Updated content...",
    "summary": "Updated",
    "has_embedding": true,
    "tags": ["auth"],
    "source_type": "jira",
    "source_ref": "PROJ-123",
    "tool": "claude",
    "recorded_at": "2026-07-22T10:05:00Z"
  },
  {
    "history_id": 41,
    "entry_id": "uuid",
    "action": "insert",
    "content": "Original content...",
    "has_embedding": true,
    "recorded_at": "2026-07-22T10:00:00Z"
  }
]
```

`action` is one of: `insert`, `update`, `delete`.

**Example:**
```bash
curl "http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/ENTRY_ID/history?limit=10" \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### POST Rollback

`POST /api/v1/hiveshares/{id}/memory/{entryId}/rollback`

**Auth:** Required
**Access:** CanWrite

Restores a memory entry to a prior version. If the history version has an embedding, it is restored directly. If not, a re-embed job is enqueued.

**Request body:**
```json
{
  "history_id": 41
}
```

**Response:** `200 OK` (returns the restored entry)

**Error responses:**
- `400` — history_id missing
- `404` — entry or history version not found

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/ENTRY_ID/rollback \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"history_id":41}'
```

---

### POST Undelete

`POST /api/v1/hiveshares/{id}/memory/undelete`

**Auth:** Required
**Access:** CanWrite

Restores a deleted memory entry from its history record. The history version must have `action: "delete"`.

**Request body:**
```json
{
  "history_id": 43
}
```

**Response:** `201 Created` (returns the restored entry with its original ID)

**Error responses:**
- `400` — history_id missing
- `404` — history version not found or not a delete action

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/memory/undelete \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"history_id":43}'
```

---

## Snapshots

### POST Create Snapshot

`POST /api/v1/hiveshares/{id}/snapshots`

**Auth:** Required
**Access:** CanWrite

Creates a point-in-time snapshot of all memory entries in the hiveshare, including their embeddings.

**Request body:**
```json
{
  "name": "before-cleanup",
  "description": "Snapshot before removing stale entries"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Snapshot name |
| `description` | No | Description |

**Response:** `201 Created`
```json
{
  "snapshot_id": 1,
  "hiveshare_id": "uuid",
  "created_by": "uuid",
  "name": "before-cleanup",
  "description": "Snapshot before removing stale entries",
  "entry_count": 15,
  "created_at": "2026-07-22T10:00:00Z"
}
```

**Error responses:**
- `400` — name missing

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/snapshots \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"before-cleanup","description":"Snapshot before removing stale entries"}'
```

---

### GET List Snapshots

`GET /api/v1/hiveshares/{id}/snapshots`

**Auth:** Required
**Access:** CanView

**Response:** `200 OK`
```json
[
  {
    "snapshot_id": 1,
    "hiveshare_id": "uuid",
    "created_by": "uuid",
    "name": "before-cleanup",
    "entry_count": 15,
    "created_at": "2026-07-22T10:00:00Z"
  }
]
```

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/snapshots \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### GET Snapshot

`GET /api/v1/hiveshares/{id}/snapshots/{snapshotId}`

**Auth:** Required
**Access:** CanView

Returns snapshot metadata and the list of frozen entries.

**Response:** `200 OK`
```json
{
  "snapshot": {
    "snapshot_id": 1,
    "hiveshare_id": "uuid",
    "created_by": "uuid",
    "name": "before-cleanup",
    "entry_count": 15,
    "created_at": "2026-07-22T10:00:00Z"
  },
  "entries": [
    {
      "entry_id": "uuid",
      "content": "...",
      "summary": "...",
      "has_embedding": true,
      "tags": ["auth"],
      "source_type": "jira",
      "source_ref": "PROJ-123",
      "tool": "claude"
    }
  ]
}
```

**Error responses:**
- `404` — snapshot not found

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/snapshots/1 \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### POST Restore Snapshot

`POST /api/v1/hiveshares/{id}/snapshots/{snapshotId}/restore`

**Auth:** Required
**Access:** CanWrite

Creates a **new hiveshare** from the snapshot. The original hiveshare is not modified. Entries with embeddings are copied as-is; entries without embeddings are queued for re-embedding.

**Request body (optional):**
```json
{
  "name": "Sprint 42 (restored)"
}
```

If `name` is omitted, defaults to `"(restored)"`.

**Response:** `201 Created`
```json
{
  "hiveshare": {
    "id": "new-uuid",
    "name": "Sprint 42 (restored)",
    "owner_id": "uuid",
    "role": "all",
    "member_count": 1,
    ...
  },
  "entries_restored": 15
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/snapshots/1/restore \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Sprint 42 (restored)"}'
```

---

### DELETE Snapshot

`DELETE /api/v1/hiveshares/{id}/snapshots/{snapshotId}`

**Auth:** Required
**Access:** CanWrite

Deletes the snapshot and all its frozen entries.

**Response:** `204 No Content`

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/snapshots/1 \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

## Metrics

### GET Hiveshare Metrics

`GET /api/v1/hiveshares/{id}/metrics`

**Auth:** Required
**Access:** CanView

**Response:** `200 OK`
```json
{
  "hiveshare": {
    "name": "Sprint 42",
    "description": "...",
    "member_count": 3
  },
  "memory": {
    "total_entries": 25,
    "by_source_type": {"jira": 15, "github_pr": 8, "manual": 2},
    "by_tool": {"claude": 20, "cursor": 3, "manual": 2},
    "unique_sources": 12
  },
  "collaboration": {
    "total_views": 150,
    "total_reuses": 45,
    "reuse_rate": 0.3,
    "top_contributors": [
      {"user_id": "uuid", "name": "Alice", "entries": 15, "reuses_received": 30}
    ]
  },
  "coverage": {
    "jira_refs_with_memory": 10,
    "github_refs_with_memory": 5
  },
  "activity": {
    "last_7d_adds": 8,
    "last_7d_searches": 25,
    "active_users_7d": 3
  }
}
```

**Example:**
```bash
curl http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/metrics \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

### GET User Metrics

`GET /api/v1/metrics/me`

**Auth:** Required

**Response:** `200 OK`
```json
{
  "total_entries": 42,
  "total_searches": 120,
  "hiveshares_owned": 3,
  "hiveshares_joined": 5,
  "total_reuses_given": 30
}
```

**Example:**
```bash
curl http://localhost:8080/api/v1/metrics/me \
  -H "Authorization: Bearer hvs_YOUR_API_KEY"
```

---

## SSE Stream

### GET Stream

`GET /api/v1/hiveshares/{id}/stream`

**Auth:** Required
**Access:** CanView

Opens a long-lived Server-Sent Events connection. Events are published via Redis pub/sub and fanned out to all connected clients.

**Headers:**
```
Accept: text/event-stream
Cache-Control: no-cache
```

**Event types:**

| Event | Payload | When |
|-------|---------|------|
| `connected` | `{"hiveshare_id": "uuid"}` | Initial connection |
| `memory_added` | Full memory entry | Entry created |
| `memory_updated` | Full memory entry | Entry updated |
| `memory_rolled_back` | Full memory entry | Entry rolled back |
| `memory_undeleted` | Full memory entry | Entry restored from deletion |

Keepalive comments (`: keepalive`) are sent every 25 seconds.

**Example:**
```bash
curl -N http://localhost:8080/api/v1/hiveshares/HIVESHARE_ID/stream \
  -H "Authorization: Bearer hvs_YOUR_API_KEY" \
  -H "Accept: text/event-stream"
```
