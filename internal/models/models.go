// Package models defines the shared data types used across all layers of the
// hiveshare service. Every type here is serialised directly to JSON for API
// responses, so field names and omitempty tags are part of the public contract.
package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered account. APIKey is only populated on
// registration — the database stores a SHA-256 hash, not the cleartext key.
type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Hiveshare is a collaborative workspace shared among a team. Role and
// MemberCount are enriched at query time and are not stored on the row.
type Hiveshare struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	OwnerID     uuid.UUID              `json:"owner_id"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	// enriched fields — not stored on the row
	Role        string `json:"role,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}

// Member represents a single user's membership in a hiveshare.
type Member struct {
	HiveshareID uuid.UUID  `json:"hiveshare_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	InvitedBy   *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt    time.Time  `json:"joined_at"`
}

// Invitation is a pending email invitation to join a hiveshare. InviteURL and
// HiveShareName are enriched at query time for the response payload.
type Invitation struct {
	ID          uuid.UUID `json:"id"`
	HiveshareID uuid.UUID `json:"hiveshare_id"`
	Email       string    `json:"email"`
	InvitedBy   uuid.UUID `json:"invited_by"`
	Token       string    `json:"token"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	// enriched fields
	InviteURL     string `json:"invite_url,omitempty"`
	HiveShareName string `json:"hiveshare_name,omitempty"`
}

// Hive is a single shared context entry saved by an agent or team member.
// SourceRef is unique within a hiveshare (enforced by DB constraint); the API
// auto-suffixes duplicates (e.g. PROJ-123-2). Score is only set on search results.
type Hive struct {
	ID          uuid.UUID              `json:"id"`
	HiveshareID uuid.UUID              `json:"hiveshare_id"`
	UserID      uuid.UUID              `json:"user_id"`
	UserName    string                 `json:"user_name,omitempty"`
	SourceType  string                 `json:"source_type"`
	SourceRef   string                 `json:"source_ref"`
	SourceURL   string                 `json:"source_url,omitempty"`
	Tool        string                 `json:"tool,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Summary     string                 `json:"summary,omitempty"`
	Tags        []string               `json:"tags"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Views       int                    `json:"views"`
	Reuses      int                    `json:"reuses"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at,omitempty"`
	Score       float64                `json:"score,omitempty"`
}

// UsageEvent records a single analytics event (add, view, search, invite_sent, etc.)
// scoped to a user and optionally to a hiveshare and a specific hive entry.
type UsageEvent struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	HiveshareID *uuid.UUID             `json:"hiveshare_id,omitempty"`
	EntryID     *uuid.UUID             `json:"entry_id,omitempty"`
	EventType   string                 `json:"event_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
}

// HiveshareMetrics is the full analytics snapshot for a hiveshare, aggregating
// hive counts, collaboration stats, source coverage, and recent activity.
type HiveshareMetrics struct {
	Hiveshare HiveShareSummary `json:"hiveshare"`
	Hive      HiveMetrics      `json:"hive"`
	Collab    CollabMetrics    `json:"collaboration"`
	Coverage  CoverageMetrics  `json:"coverage"`
	Activity  ActivityMetrics  `json:"activity"`
}

// HiveShareSummary is the lightweight hiveshare summary embedded in metrics responses.
type HiveShareSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

// HiveMetrics aggregates hive entry counts broken down by source type and tool.
type HiveMetrics struct {
	TotalEntries  int            `json:"total_entries"`
	BySourceType  map[string]int `json:"by_source_type"`
	ByTool        map[string]int `json:"by_tool"`
	UniqueSources int            `json:"unique_sources"`
}

// CollabMetrics tracks reuse and contribution patterns within a hiveshare.
type CollabMetrics struct {
	TotalViews      int               `json:"total_views"`
	TotalReuses     int               `json:"total_reuses"`
	ReuseRate       float64           `json:"reuse_rate"`
	TopContributors []ContributorStat `json:"top_contributors"`
}

// ContributorStat summarises a single user's contribution to a hiveshare.
type ContributorStat struct {
	UserID         uuid.UUID `json:"user_id"`
	Name           string    `json:"name"`
	Entries        int       `json:"entries"`
	ReusesReceived int       `json:"reuses_received"`
}

// CoverageMetrics counts how many distinct Jira and GitHub refs have at least
// one hive entry, giving teams a sense of documentation completeness.
type CoverageMetrics struct {
	JiraRefsWithMemory   int `json:"jira_refs_with_memory"`
	GithubRefsWithMemory int `json:"github_refs_with_memory"`
}

// ActivityMetrics captures event counts over the trailing 7 days.
type ActivityMetrics struct {
	Last7dAdds     int `json:"last_7d_adds"`
	Last7dSearches int `json:"last_7d_searches"`
	ActiveUsers7d  int `json:"active_users_7d"`
}

// UserMetrics aggregates per-user contribution and membership statistics.
type UserMetrics struct {
	TotalEntries     int `json:"total_entries"`
	TotalSearches    int `json:"total_searches"`
	HivsharesOwned   int `json:"hiveshares_owned"`
	HivsharesJoined  int `json:"hiveshares_joined"`
	TotalReusesGiven int `json:"total_reuses_given"`
}

// StreamEvent is the payload pushed over SSE to connected clients whenever a
// hive is added or updated in a hiveshare.
type StreamEvent struct {
	Type        string      `json:"type"`
	HiveshareID uuid.UUID   `json:"hiveshare_id"`
	Payload     interface{} `json:"payload"`
}
