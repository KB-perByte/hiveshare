package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type HiveStore struct {
	db *pgxpool.Pool
}

func NewHiveStore(db *pgxpool.Pool) *HiveStore {
	return &HiveStore{db: db}
}

type ListHiveFilter struct {
	SourceType string
	SourceRef  string
	Tag        string
	Tool       string
	Limit      int
	Offset     int
}

func (s *HiveStore) Create(ctx context.Context, e *models.Hive, embedding []float32) (*models.Hive, error) {
	var embVal interface{}
	if len(embedding) > 0 {
		embVal = pgvector.NewVector(embedding)
	}

	err := s.db.QueryRow(ctx,
		`INSERT INTO hives
		 (hiveshare_id, user_id, source_type, source_ref, source_url, tool, content, summary, embedding, tags, metadata, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id, created_at, updated_at, expires_at`,
		e.HiveshareID, e.UserID, e.SourceType, e.SourceRef, e.SourceURL,
		e.Tool, e.Content, e.Summary, embVal, e.Tags, e.Metadata, e.ExpiresAt,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt, &e.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create memory entry: %w", err)
	}
	return e, nil
}

func (s *HiveStore) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	_, err := s.db.Exec(ctx,
		`UPDATE hives SET embedding = $2, updated_at = NOW() WHERE id = $1`,
		id, pgvector.NewVector(embedding),
	)
	return err
}

func (s *HiveStore) List(ctx context.Context, hiveshareID uuid.UUID, f ListHiveFilter) ([]*models.Hive, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	wheres := []string{
		"me.hiveshare_id = $1",
		"(me.expires_at IS NULL OR me.expires_at > NOW())",
	}
	args := []interface{}{hiveshareID}
	n := 2

	if f.SourceType != "" {
		wheres = append(wheres, fmt.Sprintf("me.source_type = $%d", n))
		args = append(args, f.SourceType)
		n++
	}
	if f.SourceRef != "" {
		wheres = append(wheres, fmt.Sprintf("me.source_ref = $%d", n))
		args = append(args, f.SourceRef)
		n++
	}
	if f.Tool != "" {
		wheres = append(wheres, fmt.Sprintf("me.tool = $%d", n))
		args = append(args, f.Tool)
		n++
	}
	if f.Tag != "" {
		wheres = append(wheres, fmt.Sprintf("$%d = ANY(me.tags)", n))
		args = append(args, f.Tag)
		n++
	}

	args = append(args, f.Limit, f.Offset)
	// List omits full content to keep payloads small (content only in Get/Search).
	q := fmt.Sprintf(
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref,
		        me.summary, me.tags, me.views, me.reuses, me.created_at
		 FROM hives me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY me.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		strings.Join(wheres, " AND "), n, n+1,
	)
	return s.scanListRows(ctx, q, args...)
}

func (s *HiveStore) Get(ctx context.Context, id, hiveshareID uuid.UUID) (*models.Hive, error) {
	rows, err := s.scanRows(ctx,
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at, me.expires_at
		 FROM hives me
		 JOIN users u ON u.id = me.user_id
		 WHERE me.id = $1 AND me.hiveshare_id = $2
		   AND (me.expires_at IS NULL OR me.expires_at > NOW())`, id, hiveshareID,
	)
	if err != nil {
		return nil, fmt.Errorf("get memory entry: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("get memory entry: %w", pgx.ErrNoRows)
	}
	return rows[0], nil
}

// Update updates content, summary, tags, and optionally expires_at for a hive.
// Pass a nil expiresAt to leave the existing value unchanged.
func (s *HiveStore) Update(ctx context.Context, id, hiveshareID uuid.UUID, content, summary string, tags []string, expiresAt *time.Time) (*models.Hive, error) {
	var e models.Hive
	err := s.db.QueryRow(ctx,
		`UPDATE hives SET content = $3, summary = $4, tags = $5, embedding = NULL, updated_at = NOW(),
		                  expires_at = COALESCE($6, expires_at)
		 WHERE id = $1 AND hiveshare_id = $2
		 RETURNING id, hiveshare_id, user_id, source_type, source_ref, source_url, tool,
		           content, summary, tags, metadata, views, reuses, created_at, updated_at, expires_at`,
		id, hiveshareID, content, summary, tags, expiresAt,
	).Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.SourceType, &e.SourceRef, &e.SourceURL,
		&e.Tool, &e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses, &e.CreatedAt, &e.UpdatedAt, &e.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("update memory entry: %w", err)
	}
	return &e, nil
}

// Delete removes a hive entry. Returns (false, nil) when the entry does not
// exist in this hiveshare so the handler can return 404 without a DB error.
func (s *HiveStore) Delete(ctx context.Context, id, hiveshareID uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM hives WHERE id = $1 AND hiveshare_id = $2`, id, hiveshareID,
	)
	if err != nil {
		return false, fmt.Errorf("delete hive: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SearchVector performs cosine similarity search.
func (s *HiveStore) SearchVector(ctx context.Context, hiveshareID uuid.UUID, embedding []float32, sourceType string, limit int) ([]*models.Hive, error) {
	if limit == 0 {
		limit = 10
	}
	wheres := []string{
		"me.hiveshare_id = $1",
		"me.embedding IS NOT NULL",
		"(me.expires_at IS NULL OR me.expires_at > NOW())",
	}
	args := []interface{}{hiveshareID, pgvector.NewVector(embedding)}
	n := 3

	if sourceType != "" {
		wheres = append(wheres, fmt.Sprintf("me.source_type = $%d", n))
		args = append(args, sourceType)
		n++
	}
	args = append(args, limit)

	q := fmt.Sprintf(
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at, me.expires_at,
		        1 - (me.embedding <=> $2) AS score
		 FROM hives me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY me.embedding <=> $2
		 LIMIT $%d`,
		strings.Join(wheres, " AND "), n,
	)
	return s.scanRowsWithScore(ctx, q, args...)
}

// SearchFullText performs PostgreSQL full-text search.
// maxAgeSecs filters to entries updated within the last N seconds (0 = no filter).
func (s *HiveStore) SearchFullText(ctx context.Context, hiveshareID uuid.UUID, query, sourceType string, limit, maxAgeSecs int) ([]*models.Hive, error) {
	if limit == 0 {
		limit = 10
	}
	wheres := []string{
		"me.hiveshare_id = $1",
		"to_tsvector('english', me.content) @@ plainto_tsquery('english', $2)",
		"(me.expires_at IS NULL OR me.expires_at > NOW())",
	}
	args := []interface{}{hiveshareID, query}
	n := 3

	if sourceType != "" {
		wheres = append(wheres, fmt.Sprintf("me.source_type = $%d", n))
		args = append(args, sourceType)
		n++
	}
	if maxAgeSecs > 0 {
		wheres = append(wheres, fmt.Sprintf("me.updated_at >= NOW() - ($%d * interval '1 second')", n))
		args = append(args, maxAgeSecs)
		n++
	}
	args = append(args, limit)

	q := fmt.Sprintf(
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at, me.expires_at,
		        ts_rank(to_tsvector('english', me.content), plainto_tsquery('english', $2)) AS score
		 FROM hives me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY score DESC
		 LIMIT $%d`,
		strings.Join(wheres, " AND "), n,
	)
	return s.scanRowsWithScore(ctx, q, args...)
}

// SearchHybrid blends cosine similarity and BM25 full-text scores.
// alpha=1.0 is pure vector, alpha=0.0 is pure full-text.
// maxAgeSecs filters to entries updated within the last N seconds (0 = no filter).
func (s *HiveStore) SearchHybrid(ctx context.Context, hiveshareID uuid.UUID, embedding []float32, query, sourceType string, alpha float64, limit, maxAgeSecs int) ([]*models.Hive, error) {
	if limit == 0 {
		limit = 10
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	args := []interface{}{hiveshareID, pgvector.NewVector(embedding), query, alpha}
	n := 5

	var extraFilters []string
	if sourceType != "" {
		extraFilters = append(extraFilters, fmt.Sprintf("AND me.source_type = $%d", n))
		args = append(args, sourceType)
		n++
	}
	if maxAgeSecs > 0 {
		extraFilters = append(extraFilters, fmt.Sprintf("AND me.updated_at >= NOW() - ($%d * interval '1 second')", n))
		args = append(args, maxAgeSecs)
		n++
	}
	args = append(args, limit)
	extraClause := strings.Join(extraFilters, " ")

	q := fmt.Sprintf(`
		WITH vec AS (
			SELECT id, 1 - (embedding <=> $2) AS vscore
			FROM hives
			WHERE hiveshare_id = $1 AND embedding IS NOT NULL
			  AND (expires_at IS NULL OR expires_at > NOW())
		),
		fts AS (
			SELECT id,
				   ts_rank(to_tsvector('english', content), plainto_tsquery('english', $3)) AS fscore
			FROM hives
			WHERE hiveshare_id = $1
			  AND to_tsvector('english', content) @@ plainto_tsquery('english', $3)
			  AND (expires_at IS NULL OR expires_at > NOW())
		)
		SELECT me.id, me.hiveshare_id, me.user_id, u.name,
			   me.source_type, me.source_ref, me.source_url,
			   me.tool, me.content, me.summary, me.tags, me.metadata,
			   me.views, me.reuses, me.created_at, me.updated_at, me.expires_at,
			   COALESCE(v.vscore, 0) * $4 + COALESCE(f.fscore, 0) * (1 - $4) AS score
		FROM hives me
		JOIN users u ON u.id = me.user_id
		LEFT JOIN vec v ON v.id = me.id
		LEFT JOIN fts f ON f.id = me.id
		WHERE me.hiveshare_id = $1
		  AND (v.id IS NOT NULL OR f.id IS NOT NULL)
		  %s
		ORDER BY score DESC
		LIMIT $%d`, extraClause, n)

	return s.scanRowsWithScore(ctx, q, args...)
}

// FindSimilar returns the most similar hive to the given embedding within a
// hiveshare, optionally filtered to a specific source_ref. Returns nil when
// no result exceeds the similarity threshold.
func (s *HiveStore) FindSimilar(ctx context.Context, hiveshareID uuid.UUID, sourceRef string, embedding []float32, threshold float64) (*models.Hive, error) {
	rows, err := s.scanRowsWithScore(ctx,
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name,
		        me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata,
		        me.views, me.reuses, me.created_at, me.updated_at, me.expires_at,
		        1 - (me.embedding <=> $3) AS score
		 FROM hives me
		 JOIN users u ON u.id = me.user_id
		 WHERE me.hiveshare_id = $1
		   AND me.source_ref = $2
		   AND me.embedding IS NOT NULL
		   AND (me.expires_at IS NULL OR me.expires_at > NOW())
		   AND 1 - (me.embedding <=> $3) >= $4
		 ORDER BY score DESC
		 LIMIT 1`,
		hiveshareID, sourceRef, pgvector.NewVector(embedding), threshold,
	)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func (s *HiveStore) IncrementReuse(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE hives SET reuses = reuses + 1 WHERE id = $1`, id)
	return err
}

func (s *HiveStore) scanListRows(ctx context.Context, q string, args ...interface{}) ([]*models.Hive, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Hive
	for rows.Next() {
		var e models.Hive
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.Summary, &e.Tags, &e.Views, &e.Reuses, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}

func (s *HiveStore) scanRows(ctx context.Context, q string, args ...interface{}) ([]*models.Hive, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Hive
	for rows.Next() {
		var e models.Hive
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool,
			&e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
			&e.CreatedAt, &e.UpdatedAt, &e.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}

func (s *HiveStore) scanRowsWithScore(ctx context.Context, q string, args ...interface{}) ([]*models.Hive, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Hive
	for rows.Next() {
		var e models.Hive
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool,
			&e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
			&e.CreatedAt, &e.UpdatedAt, &e.ExpiresAt, &e.Score); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}
