package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KB-perByte/hiveshare/internal/models"
)

type MetricsStore struct {
	db *pgxpool.Pool
}

func NewMetricsStore(db *pgxpool.Pool) *MetricsStore {
	return &MetricsStore{db: db}
}

func (s *MetricsStore) RecordEvent(ctx context.Context, ev *models.UsageEvent) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO usage_events (user_id, hiveshare_id, entry_id, event_type, metadata)
		 VALUES ($1, $2, $3, $4, $5)`,
		ev.UserID, ev.HiveshareID, ev.EntryID, ev.EventType, ev.Metadata,
	)
	return err
}

func (s *MetricsStore) HiveshareMetrics(ctx context.Context, hsID uuid.UUID) (*models.HiveshareMetrics, error) {
	m := &models.HiveshareMetrics{}

	// hiveshare summary
	if err := s.db.QueryRow(ctx,
		`SELECT h.name, COALESCE(h.description,''), COUNT(hm.user_id)
		 FROM hiveshares h
		 JOIN hiveshare_members hm ON hm.hiveshare_id = h.id
		 WHERE h.id = $1
		 GROUP BY h.name, h.description`, hsID,
	).Scan(&m.Hiveshare.Name, &m.Hiveshare.Description, &m.Hiveshare.MemberCount); err != nil {
		return nil, err
	}

	// memory counts
	m.Memory.BySourceType = make(map[string]int)
	m.Memory.ByTool = make(map[string]int)

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT source_ref) FROM memory_entries WHERE hiveshare_id = $1`, hsID,
	).Scan(&m.Memory.TotalEntries, &m.Memory.UniqueSources); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT source_type, COUNT(*) FROM memory_entries WHERE hiveshare_id = $1 GROUP BY source_type`, hsID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var v int
		rows.Scan(&k, &v)
		m.Memory.BySourceType[k] = v
	}
	rows.Close()

	rows, err = s.db.Query(ctx,
		`SELECT tool, COUNT(*) FROM memory_entries WHERE hiveshare_id = $1 GROUP BY tool`, hsID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var v int
		rows.Scan(&k, &v)
		m.Memory.ByTool[k] = v
	}
	rows.Close()

	// collab metrics
	if err := s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(views),0), COALESCE(SUM(reuses),0) FROM memory_entries WHERE hiveshare_id = $1`, hsID,
	).Scan(&m.Collab.TotalViews, &m.Collab.TotalReuses); err != nil {
		return nil, err
	}
	if m.Collab.TotalViews > 0 {
		m.Collab.ReuseRate = float64(m.Collab.TotalReuses) / float64(m.Collab.TotalViews)
	}

	// top contributors
	rows, err = s.db.Query(ctx,
		`SELECT u.id, u.name, COUNT(me.id) AS entries, COALESCE(SUM(me.reuses),0) AS reuses_received
		 FROM memory_entries me
		 JOIN users u ON u.id = me.user_id
		 WHERE me.hiveshare_id = $1
		 GROUP BY u.id, u.name
		 ORDER BY entries DESC
		 LIMIT 5`, hsID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c models.ContributorStat
		rows.Scan(&c.UserID, &c.Name, &c.Entries, &c.ReusesReceived)
		m.Collab.TopContributors = append(m.Collab.TopContributors, c)
	}
	rows.Close()

	// coverage
	if err := s.db.QueryRow(ctx,
		`SELECT
		  COUNT(*) FILTER (WHERE source_type = 'jira'),
		  COUNT(*) FILTER (WHERE source_type IN ('github_issue','github_pr'))
		 FROM (SELECT DISTINCT source_ref, source_type FROM memory_entries WHERE hiveshare_id = $1) AS t`, hsID,
	).Scan(&m.Coverage.JiraRefsWithMemory, &m.Coverage.GithubRefsWithMemory); err != nil {
		return nil, err
	}

	// activity
	since7d := time.Now().AddDate(0, 0, -7)
	if err := s.db.QueryRow(ctx,
		`SELECT
		  COUNT(*) FILTER (WHERE event_type = 'add'),
		  COUNT(*) FILTER (WHERE event_type = 'search'),
		  COUNT(DISTINCT user_id)
		 FROM usage_events
		 WHERE hiveshare_id = $1 AND created_at >= $2`, hsID, since7d,
	).Scan(&m.Activity.Last7dAdds, &m.Activity.Last7dSearches, &m.Activity.ActiveUsers7d); err != nil {
		return nil, err
	}

	return m, nil
}

func (s *MetricsStore) UserMetrics(ctx context.Context, userID uuid.UUID) (*models.UserMetrics, error) {
	m := &models.UserMetrics{}

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM memory_entries WHERE user_id = $1`, userID,
	).Scan(&m.TotalEntries); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM usage_events WHERE user_id = $1 AND event_type = 'search'`, userID,
	).Scan(&m.TotalSearches); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM hiveshares WHERE owner_id = $1`, userID,
	).Scan(&m.HivsharesOwned); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM hiveshare_members WHERE user_id = $1`, userID,
	).Scan(&m.HivsharesJoined); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(reuses),0) FROM memory_entries WHERE user_id = $1`, userID,
	).Scan(&m.TotalReusesGiven); err != nil {
		return nil, err
	}

	return m, nil
}

// PurgeOldUsageEvents deletes usage_events older than retention (rolling TTL).
func (s *MetricsStore) PurgeOldUsageEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := s.db.Exec(ctx, `DELETE FROM usage_events WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
