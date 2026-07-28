# HiveShare — Interaction Tutorial

A hands-on walkthrough of every major flow. Run these in order — each section
builds on the previous one. All curl examples point at the EC2 demo server;
swap in your own `HIVESHARE_SERVER_URL` and `HIVESHARE_API_KEY`.

Set these once to avoid repeating them:

```bash
export SERVER="http://ec2-52-203-192-167.compute-1.amazonaws.com:8080"
export KEY="hvs_da3776b7243f8e02387f82930ddad83a62547140149c9504"
export HS="6b46fb3a-67a6-4c7a-a486-2137191ede63"
```

---

## 1 — Verify the server is healthy

```bash
curl "$SERVER/health"
```

**Expected:**
```json
{
  "status": "ok",
  "db": "ok",
  "redis": "ok",
  "commit": "f290572",
  "build_time": "2026-07-28T11:08:44Z"
}
```

If `db` or `redis` shows `unavailable`, the backing services are down — check
`docker compose ps` on the server.

---

## 2 — Verify MCP is connected (Claude Code / Cursor)

Tell your agent:

```
List my hiveshares and tell me which one is currently active.
```

**Expected:** Claude calls `list_hiveshares` and returns something like:

```
You have 1 hiveshare:
- test-hive (6b46fb3a-...) — role: all, 2 members
The active default is test-hive.
```

**If it says "connection refused":**
- Run `curl "$SERVER/health"` to confirm the server is reachable
- Restart Claude Code (the MCP process may be stale from before today's config changes)
- Check `~/.claude/claude_desktop_config.json` has `HIVESHARE_SERVER_URL` and `HIVESHARE_DEFAULT_HIVESHARE` set

---

## 3 — Save your first hive

### Via CLI

```bash
echo "PROJ-1: Payment service times out after 30s when the downstream currency
API is slow. Root cause: no circuit breaker. Fix: add Hystrix with 5s timeout
and fallback to cached exchange rate." | \
  hiveshare hive add \
    --source-type jira \
    --source-ref PROJ-1 \
    --summary "Payment timeout — missing circuit breaker, fix with Hystrix" \
    --tool claude
```

### Via curl

```bash
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "source_type": "jira",
    "source_ref": "PROJ-1",
    "content": "Payment service times out after 30s when the downstream currency API is slow. Root cause: no circuit breaker. Fix: add Hystrix with 5s timeout and fallback to cached exchange rate.",
    "summary": "Payment timeout — missing circuit breaker, fix with Hystrix",
    "tool": "claude",
    "tags": ["payment", "reliability"]
  }'
```

**Expected:** `201 Created` with the hive object. Note the `id` — you will use it below.

```json
{
  "id": "a1b2c3d4-...",
  "source_ref": "PROJ-1",
  "summary": "Payment timeout — missing circuit breaker, fix with Hystrix",
  "views": 0,
  "reuses": 0
}
```

### Via agent (MCP)

```
Save this to the hiveshare:
source_type: jira
source_ref: PROJ-2
content: "Auth middleware refactor. JWT RS256 replaces session tokens.
Token validation must happen before forwarding to internal services."
summary: "Auth refactor — JWT RS256, validate before forwarding"
```

**Expected:** Claude calls `add_hive`, returns the created entry. The embedding
is queued async — search will use full-text immediately and switch to vector
search once the embedding worker processes the job (usually within seconds).

---

## 4 — Retrieve context for a specific ticket

### CLI

```bash
hiveshare hive search "PROJ-1" --source-type jira
```

### curl

```bash
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives/search" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "PROJ-1", "source_type": "jira", "limit": 5}'
```

### Agent

```
What do we know about PROJ-1?
```

**Expected:** Claude calls `get_context("PROJ-1")` first (exact match), falls
back to `search_hives` if needed. Returns the summary and full content.

Check the `type` field in the search response — it will be `"fulltext"` until
the embedding is ready, then `"hybrid"` after. The hybrid score blends cosine
similarity (70%) with BM25 full-text (30%) by default.

---

## 5 — Search — verify hybrid scoring

### curl

```bash
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives/search" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "payment service timeout reliability", "alpha": 0.7, "limit": 5}'
```

**Expected:**

```json
{
  "results": [
    {
      "source_ref": "PROJ-1",
      "summary": "Payment timeout — missing circuit breaker",
      "score": 0.82,
      ...
    }
  ],
  "count": 1,
  "query": "payment service timeout reliability",
  "type": "hybrid"
}
```

`type: "hybrid"` means the CTE blended HNSW cosine + BM25. Try `"alpha": 0.0`
to force full-text only, or `"alpha": 1.0` for pure vector.

### Agent

```
Search the hiveshare for anything related to payment timeouts.
Show me the score and what search type was used.
```

---

## 6 — Invite a teammate

### CLI

```bash
hiveshare invite mac@demo.com --role all
```

**Expected output:**

```
Invitation sent to mac@demo.com
Role: all | Expires: 2026-08-04T...

Invite link : http://ec2-.../api/v1/invitations/<token>/accept
Token       : <token>

── How to accept ────────────────────────────────────────

  CLI / curl (get an API key):
    curl -X POST 'http://ec2-.../api/v1/invitations/<token>/accept' \
      -H 'Content-Type: application/json' \
      -d '{"name": "Your Name"}'

  MCP / Claude Code / Cursor (no terminal needed):
    Tell your agent: "Accept my hiveshare invite, token is <token>, name is Your Name"
```

Note the token — you need it for the next section.

---

## 7 — Accept the invite (three ways)

Pick the path that matches the teammate's setup.

### Option A — curl (CLI user, gets API key immediately)

```bash
curl -X POST "$SERVER/api/v1/invitations/<token>/accept" \
  -H "Content-Type: application/json" \
  -d '{"name": "Mac"}'
```

**Expected:**

```json
{
  "message": "Welcome to test-hive",
  "hiveshare_id": "6b46fb3a-...",
  "user": {
    "id": "...",
    "email": "mac@demo.com",
    "name": "Mac",
    "api_key": "hvs_NEW_KEY_HERE",
    "created_at": "..."
  }
}
```

Save `api_key` — it is shown **only once**. The server stores a SHA-256 hash.

### Option B — Agent / MCP (MCP-only user)

The teammate starts `hiveshare-mcp` with only `HIVESHARE_SERVER_URL` set (no
API key yet). Then they tell Claude:

```
Accept my hiveshare invite. The token is <token> and my name is Mac.
```

Claude calls `accept_invite` and returns:

```
New account created.
1) Set HIVESHARE_API_KEY=hvs_... in your MCP config.
2) Set HIVESHARE_DEFAULT_HIVESHARE=6b46fb3a-... in your MCP config.
3) Restart Claude/Cursor. You will then have full access.
```

### Option C — Existing user joining a new hiveshare

If Mac already has an account and API key, they can accept via curl with the
same command as Option A. The response will NOT include `api_key` (no new
account created). Claude's `accept_invite` will return:

```
You have been added to hiveshare 6b46fb3a-...
Run: hiveshare use 6b46fb3a-... (or set HIVESHARE_DEFAULT_HIVESHARE and restart).
Your existing API key is unchanged.
```

---

## 8 — Verify the new user can access the hiveshare

```bash
export KEY2="hvs_NEW_KEY_FROM_STEP_7"

# Confirm identity
curl "$SERVER/api/v1/auth/whoami" \
  -H "Authorization: Bearer $KEY2"

# List hiveshares — should include test-hive
curl "$SERVER/api/v1/hiveshares" \
  -H "Authorization: Bearer $KEY2"

# Search as user 2 — finds hives added by user 1
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives/search" \
  -H "Authorization: Bearer $KEY2" \
  -H "Content-Type: application/json" \
  -d '{"query": "circuit breaker", "limit": 3}'
```

**Expected:** user 2 finds PROJ-1 created by user 1 — cross-user retrieval
working.

---

## 9 — Collaboration round-trip (two users)

Add context as user 2, retrieve as user 1.

```bash
# User 2 adds a hive
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives" \
  -H "Authorization: Bearer $KEY2" \
  -H "Content-Type: application/json" \
  -d '{
    "source_type": "github_pr",
    "source_ref": "myrepo#99",
    "content": "PR #99 refactors the auth middleware. Removes legacy session tokens, adds JWT with RS256. Token validation must happen before forwarding to internal services.",
    "summary": "Auth middleware — JWT RS256 replaces session tokens",
    "tool": "cursor",
    "tags": ["auth", "jwt"]
  }'

# User 1 searches and finds user 2's hive
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives/search" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "JWT authentication refactor", "limit": 5}'
```

**Expected:** user 1's search returns user 2's PR hive with a non-zero score,
`user_name` field shows "Mac". This is the core collaboration loop.

### Agent version

User 1 tells Claude:

```
Before you answer anything about our auth service,
search the hiveshare for existing context.
```

Claude calls `search_hives("auth service")` and surfaces Mac's PR summary
without re-reading the PR.

---

## 10 — Real-time stream

Open two terminals.

**Terminal 1 — watch the stream:**

```bash
hiveshare stream
```

Or via curl (raw SSE):

```bash
curl -N "$SERVER/api/v1/hiveshares/$HS/stream" \
  -H "Authorization: Bearer $KEY" \
  -H "Accept: text/event-stream"
```

**Terminal 2 — add a hive:**

```bash
hiveshare hive add \
  --source-type manual \
  --source-ref "stream-test-$(date +%s)" \
  --content "testing live stream"
```

**Expected in terminal 1 within ~1 second:**

```
[14:23:01] + hive added: manual/stream-test-1722167... by sagpaul
```

The stream uses Server-Sent Events. One Redis pub/sub subscription is shared
across all local SSE clients for the same hiveshare — no per-client Redis
overhead.

---

## 11 — History and rollback

```bash
# Get the PROJ-1 entry ID (from step 3 or search)
ENTRY_ID="a1b2c3d4-..."

# Check history — should have one insert row
curl "$SERVER/api/v1/hiveshares/$HS/hives/$ENTRY_ID/history" \
  -H "Authorization: Bearer $KEY"
```

**Expected:**

```json
[
  {
    "history_id": 1,
    "action": "insert",
    "summary": "Payment timeout — missing circuit breaker",
    "has_embedding": false,
    "recorded_at": "2026-07-28T..."
  }
]
```

Update the hive and check history grows:

```bash
curl -X PUT "$SERVER/api/v1/hiveshares/$HS/hives/$ENTRY_ID" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Payment service times out after 30s. Root cause: no circuit breaker. Fix: Hystrix with 5s timeout. RESOLVED in v2.4.1 — deployed 2026-07-28.",
    "summary": "Payment timeout — RESOLVED in v2.4.1",
    "tags": ["payment", "reliability", "resolved"]
  }'

# History now has insert + update
curl "$SERVER/api/v1/hiveshares/$HS/hives/$ENTRY_ID/history" \
  -H "Authorization: Bearer $KEY"
```

Rollback to the original:

```bash
# Use history_id from the first (insert) row
curl -X POST "$SERVER/api/v1/hiveshares/$HS/hives/$ENTRY_ID/rollback" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"history_id": 1}'
```

**Expected:** content reverts to the original. A new history row with `action: "update"` is added (the rollback itself is recorded).

---

## 12 — Rate limit verification

```bash
# Hit search 35 times — should 429 after 30
for i in $(seq 1 35); do
  code=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$SERVER/api/v1/hiveshares/$HS/hives/search" \
    -H "Authorization: Bearer $KEY" \
    -H "Content-Type: application/json" \
    -d '{"query":"rate limit test"}')
  echo "$i: $code"
done
```

**Expected:** requests 1–30 return `200`, 31–35 return `429 Too Many Requests`.

Wait 60 seconds and it resets. Rate limits are per key per endpoint:

| Endpoint | Limit |
|---|---|
| `POST /auth/register`, invite accept | 10/min by IP |
| Hive create / update / delete / rollback | 20/min by key |
| `POST /hives/search` | 30/min by key |
| Everything else | 200/min by key |

---

## 13 — Metrics

```bash
# Personal stats
curl "$SERVER/api/v1/metrics/me" \
  -H "Authorization: Bearer $KEY"

# Hiveshare stats
curl "$SERVER/api/v1/hiveshares/$HS/metrics" \
  -H "Authorization: Bearer $KEY"
```

**Expected hiveshare metrics:**

```json
{
  "hiveshare": { "name": "test-hive", "member_count": 2 },
  "hive": {
    "total_entries": 3,
    "by_source_type": { "jira": 1, "github_pr": 1, "manual": 1 }
  },
  "collaboration": {
    "total_views": 4,
    "reuse_rate": 0.0
  },
  "activity": {
    "last_7d_adds": 3,
    "last_7d_searches": 8,
    "active_users_7d": 2
  }
}
```

### Agent

```
Show me the hiveshare metrics. How many entries do we have and
who are the top contributors?
```

---

## 14 — Snapshot and restore

```bash
# Take a snapshot
curl -X POST "$SERVER/api/v1/hiveshares/$HS/snapshots" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "tutorial-checkpoint", "description": "End of tutorial state"}'

# Note the snapshot_id from the response, e.g. 1

# List snapshots
curl "$SERVER/api/v1/hiveshares/$HS/snapshots" \
  -H "Authorization: Bearer $KEY"

# Restore to a NEW hiveshare (original untouched)
curl -X POST "$SERVER/api/v1/hiveshares/$HS/snapshots/1/restore" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "tutorial-restored"}'
```

**Expected:** a new hiveshare UUID is created with all entries copied. The
original `test-hive` is unchanged. Switch to the restored hiveshare with
`hiveshare use <new-uuid>`.

---

## Quick checklist

Run through this after any fresh install to confirm everything is wired up:

```
[ ] curl $SERVER/health                          → status: ok, db: ok, redis: ok
[ ] hiveshare auth status                        → logged in as sagpaul
[ ] hiveshare list                               → test-hive visible with role: all
[ ] hiveshare hive add (one entry)               → 201, entry id returned
[ ] hiveshare hive search "circuit breaker"      → finds the entry
[ ] hiveshare stream (open)                      → connected event received
[ ] hiveshare hive add second entry              → stream shows hive_added within 1s
[ ] MCP: "List my hiveshares"                    → returns test-hive
[ ] MCP: "What do we know about PROJ-1?"         → returns summary without re-reading
[ ] MCP: "Add a hive for PROJ-3"                 → add_hive called, 201 returned
[ ] hiveshare metrics                            → total_entries ≥ 2, active_users_7d ≥ 1
```
