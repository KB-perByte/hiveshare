package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/KB-perByte/hiveshare/internal/models"
)

type MemoryStore struct {
	db *pgxpool.Pool
}

func NewMemoryStore(db *pgxpool.Pool) *MemoryStore {
	return &MemoryStore{db: db}
}

type ListMemoryFilter struct {
	SourceType string
	SourceRef  string
	Tag        string
	Tool       string
	Limit      int
	Offset     int
}

func (s *MemoryStore) Create(ctx context.Context, e *models.MemoryEntry, embedding []float32) (*models.MemoryEntry, error) {
	var embVal interface{}
	if len(embedding) > 0 {
		embVal = pgvector.NewVector(embedding)
	}

	err := s.db.QueryRow(ctx,
		`INSERT INTO memory_entries
		 (hiveshare_id, user_id, source_type, source_ref, source_url, tool, content, summary, embedding, tags, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id, created_at, updated_at`,
		e.HiveshareID, e.UserID, e.SourceType, e.SourceRef, e.SourceURL,
		e.Tool, e.Content, e.Summary, embVal, e.Tags, e.Metadata,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create memory entry: %w", err)
	}
	return e, nil
}

func (s *MemoryStore) UpdateEmbedding(ctx context.Context, id uuid.UUID, embedding []float32) error {
	_, err := s.db.Exec(ctx,
		`UPDATE memory_entries SET embedding = $2, updated_at = NOW() WHERE id = $1`,
		id, pgvector.NewVector(embedding),
	)
	return err
}

func (s *MemoryStore) List(ctx context.Context, hiveshareID uuid.UUID, f ListMemoryFilter) ([]*models.MemoryEntry, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	wheres := []string{"me.hiveshare_id = $1"}
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
		 FROM memory_entries me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY me.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		strings.Join(wheres, " AND "), n, n+1,
	)
	return s.scanListRows(ctx, q, args...)
}

func (s *MemoryStore) Get(ctx context.Context, id, hiveshareID uuid.UUID) (*models.MemoryEntry, error) {
	rows, err := s.scanRows(ctx,
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at
		 FROM memory_entries me
		 JOIN users u ON u.id = me.user_id
		 WHERE me.id = $1 AND me.hiveshare_id = $2`, id, hiveshareID,
	)
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("get memory entry: %w", err)
	}
	return rows[0], nil
}

func (s *MemoryStore) Update(ctx context.Context, id, hiveshareID uuid.UUID, content, summary string, tags []string) (*models.MemoryEntry, error) {
	var e models.MemoryEntry
	err := s.db.QueryRow(ctx,
		`UPDATE memory_entries SET content = $3, summary = $4, tags = $5, embedding = NULL, updated_at = NOW()
		 WHERE id = $1 AND hiveshare_id = $2
		 RETURNING id, hiveshare_id, user_id, source_type, source_ref, source_url, tool,
		           content, summary, tags, metadata, views, reuses, created_at, updated_at`,
		id, hiveshareID, content, summary, tags,
	).Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.SourceType, &e.SourceRef, &e.SourceURL,
		&e.Tool, &e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update memory entry: %w", err)
	}
	return &e, nil
}

func (s *MemoryStore) Delete(ctx context.Context, id, hiveshareID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM memory_entries WHERE id = $1 AND hiveshare_id = $2`, id, hiveshareID,
	)
	return err
}

// SearchVector performs cosine similarity search.
func (s *MemoryStore) SearchVector(ctx context.Context, hiveshareID uuid.UUID, embedding []float32, sourceType string, limit int) ([]*models.MemoryEntry, error) {
	if limit == 0 {
		limit = 10
	}
	wheres := []string{"me.hiveshare_id = $1", "me.embedding IS NOT NULL"}
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
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at,
		        1 - (me.embedding <=> $2) AS score
		 FROM memory_entries me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY me.embedding <=> $2
		 LIMIT $%d`,
		strings.Join(wheres, " AND "), n,
	)
	return s.scanRowsWithScore(ctx, q, args...)
}

// SearchFullText performs PostgreSQL full-text search.
func (s *MemoryStore) SearchFullText(ctx context.Context, hiveshareID uuid.UUID, query, sourceType string, limit int) ([]*models.MemoryEntry, error) {
	if limit == 0 {
		limit = 10
	}
	wheres := []string{
		"me.hiveshare_id = $1",
		"to_tsvector('english', me.content) @@ plainto_tsquery('english', $2)",
	}
	args := []interface{}{hiveshareID, query}
	n := 3

	if sourceType != "" {
		wheres = append(wheres, fmt.Sprintf("me.source_type = $%d", n))
		args = append(args, sourceType)
		n++
	}
	args = append(args, limit)

	q := fmt.Sprintf(
		`SELECT me.id, me.hiveshare_id, me.user_id, u.name, me.source_type, me.source_ref, me.source_url,
		        me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses, me.created_at, me.updated_at,
		        ts_rank(to_tsvector('english', me.content), plainto_tsquery('english', $2)) AS score
		 FROM memory_entries me
		 JOIN users u ON u.id = me.user_id
		 WHERE %s
		 ORDER BY score DESC
		 LIMIT $%d`,
		strings.Join(wheres, " AND "), n,
	)
	return s.scanRowsWithScore(ctx, q, args...)
}

func (s *MemoryStore) IncrementReuse(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE memory_entries SET reuses = reuses + 1 WHERE id = $1`, id)
	return err
}

func (s *MemoryStore) scanListRows(ctx context.Context, q string, args ...interface{}) ([]*models.MemoryEntry, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.MemoryEntry
	for rows.Next() {
		var e models.MemoryEntry
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.Summary, &e.Tags, &e.Views, &e.Reuses, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}

func (s *MemoryStore) scanRows(ctx context.Context, q string, args ...interface{}) ([]*models.MemoryEntry, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.MemoryEntry
	for rows.Next() {
		var e models.MemoryEntry
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool,
			&e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}

func (s *MemoryStore) scanRowsWithScore(ctx context.Context, q string, args ...interface{}) ([]*models.MemoryEntry, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.MemoryEntry
	for rows.Next() {
		var e models.MemoryEntry
		if err := rows.Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.UserName,
			&e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool,
			&e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
			&e.CreatedAt, &e.UpdatedAt, &e.Score); err != nil {
			return nil, err
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}
