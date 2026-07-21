package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Hiveshare struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	OwnerID     uuid.UUID              `json:"owner_id"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	// enriched fields
	Role        string `json:"role,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}

type Member struct {
	HiveshareID uuid.UUID  `json:"hiveshare_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	InvitedBy   *uuid.UUID `json:"invited_by,omitempty"`
	JoinedAt    time.Time  `json:"joined_at"`
}

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
	// enriched
	InviteURL     string `json:"invite_url,omitempty"`
	HiveShareName string `json:"hiveshare_name,omitempty"`
}

type MemoryEntry struct {
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
	UpdatedAt   time.Time              `json:"updated_at,omitempty"`
	// search result field
	Score float64 `json:"score,omitempty"`
}

type UsageEvent struct {
	ID          uuid.UUID              `json:"id"`
	UserID      uuid.UUID              `json:"user_id"`
	HiveshareID *uuid.UUID             `json:"hiveshare_id,omitempty"`
	EntryID     *uuid.UUID             `json:"entry_id,omitempty"`
	EventType   string                 `json:"event_type"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
}

// Metrics aggregates

type HiveshareMetrics struct {
	Hiveshare   HiveShareSummary    `json:"hiveshare"`
	Memory      MemoryMetrics       `json:"memory"`
	Collab      CollabMetrics       `json:"collaboration"`
	Coverage    CoverageMetrics     `json:"coverage"`
	Activity    ActivityMetrics     `json:"activity"`
}

type HiveShareSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MemberCount int    `json:"member_count"`
}

type MemoryMetrics struct {
	TotalEntries  int            `json:"total_entries"`
	BySourceType  map[string]int `json:"by_source_type"`
	ByTool        map[string]int `json:"by_tool"`
	UniqueSources int            `json:"unique_sources"`
}

type CollabMetrics struct {
	TotalViews      int               `json:"total_views"`
	TotalReuses     int               `json:"total_reuses"`
	ReuseRate       float64           `json:"reuse_rate"`
	TopContributors []ContributorStat `json:"top_contributors"`
}

type ContributorStat struct {
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	Entries       int       `json:"entries"`
	ReusesReceived int      `json:"reuses_received"`
}

type CoverageMetrics struct {
	JiraRefsWithMemory   int `json:"jira_refs_with_memory"`
	GithubRefsWithMemory int `json:"github_refs_with_memory"`
}

type ActivityMetrics struct {
	Last7dAdds     int `json:"last_7d_adds"`
	Last7dSearches int `json:"last_7d_searches"`
	ActiveUsers7d  int `json:"active_users_7d"`
}

type UserMetrics struct {
	TotalEntries    int `json:"total_entries"`
	TotalSearches   int `json:"total_searches"`
	HivsharesOwned int `json:"hiveshares_owned"`
	HivsharesJoined int `json:"hiveshares_joined"`
	TotalReusesGiven int `json:"total_reuses_given"`
}

// SSE event payload

type StreamEvent struct {
	Type        string      `json:"type"`
	HiveshareID uuid.UUID   `json:"hiveshare_id"`
	Payload     interface{} `json:"payload"`
}
