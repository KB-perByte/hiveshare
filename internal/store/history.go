package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/KB-perByte/hiveshare/internal/models"
)

// ErrForbidden is returned when the caller lacks membership on a source hiveshare.
var ErrForbidden = errors.New("forbidden")

type HistoryStore struct {
	db *pgxpool.Pool
}

func NewHistoryStore(db *pgxpool.Pool) *HistoryStore {
	return &HistoryStore{db: db}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ── Per-entry history ────────────────────────────────────────────────────────

func (s *HistoryStore) ListVersions(ctx context.Context, entryID, hiveshareID uuid.UUID, limit, offset int) ([]*models.HistoryEntry, error) {
	if limit == 0 {
		limit = 20
	}
	rows, err := s.db.Query(ctx,
		`SELECT history_id, entry_id, hiveshare_id, user_id, action,
		        content, summary, (embedding IS NOT NULL) AS has_embedding,
		        tags, metadata, source_type, source_ref, source_url, tool, recorded_at
		 FROM hives_history
		 WHERE entry_id = $1 AND hiveshare_id = $2
		 ORDER BY recorded_at DESC
		 LIMIT $3 OFFSET $4`,
		entryID, hiveshareID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()

	var result []*models.HistoryEntry
	for rows.Next() {
		var h models.HistoryEntry
		if err := rows.Scan(&h.HistoryID, &h.EntryID, &h.HiveshareID, &h.UserID, &h.Action,
			&h.Content, &h.Summary, &h.HasEmbedding,
			&h.Tags, &h.Metadata, &h.SourceType, &h.SourceRef, &h.SourceURL, &h.Tool, &h.RecordedAt); err != nil {
			return nil, err
		}
		result = append(result, &h)
	}
	return result, rows.Err()
}

func (s *HistoryStore) Rollback(ctx context.Context, entryID, hiveshareID uuid.UUID, historyID int64) (*models.Hive, bool, error) {
	var e models.Hive
	var hasEmbedding bool
	err := s.db.QueryRow(ctx,
		`UPDATE hives me
		 SET content = h.content, summary = h.summary, tags = h.tags, metadata = h.metadata,
		     embedding = h.embedding, updated_at = NOW()
		 FROM hives_history h
		 WHERE me.id = h.entry_id
		   AND me.id = $1 AND me.hiveshare_id = $2 AND h.history_id = $3
		 RETURNING me.id, me.hiveshare_id, me.user_id, me.source_type, me.source_ref, me.source_url,
		           me.tool, me.content, me.summary, me.tags, me.metadata, me.views, me.reuses,
		           me.created_at, me.updated_at, (me.embedding IS NOT NULL)`,
		entryID, hiveshareID, historyID,
	).Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.SourceType, &e.SourceRef, &e.SourceURL,
		&e.Tool, &e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
		&e.CreatedAt, &e.UpdatedAt, &hasEmbedding)
	if err != nil {
		return nil, false, fmt.Errorf("rollback entry: %w", err)
	}
	return &e, hasEmbedding, nil
}

func (s *HistoryStore) Undelete(ctx context.Context, historyID int64, hiveshareID uuid.UUID) (*models.Hive, bool, error) {
	var src models.HistoryEntry
	err := s.db.QueryRow(ctx,
		`SELECT entry_id, hiveshare_id, user_id, content, summary, embedding IS NOT NULL,
		        tags, metadata, source_type, source_ref, source_url, tool
		 FROM hives_history
		 WHERE history_id = $1 AND action = 'delete' AND hiveshare_id = $2`,
		historyID, hiveshareID,
	).Scan(&src.EntryID, &src.HiveshareID, &src.UserID, &src.Content, &src.Summary, &src.HasEmbedding,
		&src.Tags, &src.Metadata, &src.SourceType, &src.SourceRef, &src.SourceURL, &src.Tool)
	if err != nil {
		return nil, false, fmt.Errorf("undelete entry: %w", err)
	}

	baseRef := src.SourceRef
	var e models.Hive
	var hasEmbedding bool
	for n := 0; ; n++ {
		ref := baseRef
		if n > 0 {
			ref = fmt.Sprintf("%s-%d", baseRef, n+1)
		}
		err = s.db.QueryRow(ctx,
			`INSERT INTO hives
			     (id, hiveshare_id, user_id, source_type, source_ref, source_url,
			      tool, content, summary, embedding, tags, metadata)
			 SELECT entry_id, hiveshare_id, user_id, source_type, $3, source_url,
			        tool, content, summary, embedding, tags, metadata
			 FROM hives_history
			 WHERE history_id = $1 AND action = 'delete' AND hiveshare_id = $2
			 RETURNING id, hiveshare_id, user_id, source_type, source_ref, source_url,
			           tool, content, summary, tags, metadata, views, reuses,
			           created_at, updated_at, (embedding IS NOT NULL)`,
			historyID, hiveshareID, ref,
		).Scan(&e.ID, &e.HiveshareID, &e.UserID, &e.SourceType, &e.SourceRef, &e.SourceURL,
			&e.Tool, &e.Content, &e.Summary, &e.Tags, &e.Metadata, &e.Views, &e.Reuses,
			&e.CreatedAt, &e.UpdatedAt, &hasEmbedding)
		if err == nil {
			return &e, hasEmbedding, nil
		}
		if n >= 9 || !isUniqueViolation(err) {
			return nil, false, fmt.Errorf("undelete entry: %w", err)
		}
	}
}

// ── Purge ────────────────────────────────────────────────────────────────────

func (s *HistoryStore) PurgeByAge(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := s.db.Exec(ctx,
		`DELETE FROM hives_history WHERE recorded_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("purge history by age: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *HistoryStore) PurgeByCount(ctx context.Context, maxVersions int) (int64, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM hives_history
		 WHERE history_id IN (
		     SELECT history_id FROM (
		         SELECT history_id,
		                ROW_NUMBER() OVER (PARTITION BY entry_id ORDER BY recorded_at DESC) AS rn
		         FROM hives_history
		     ) ranked
		     WHERE rn > $1
		 )`,
		maxVersions,
	)
	if err != nil {
		return 0, fmt.Errorf("purge history by count: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── Snapshots ────────────────────────────────────────────────────────────────

func (s *HistoryStore) CreateSnapshot(ctx context.Context, hiveshareID, userID uuid.UUID, name, description string) (*models.Snapshot, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var snap models.Snapshot
	err = tx.QueryRow(ctx,
		`INSERT INTO hiveshare_snapshots (hiveshare_id, created_by, name, description)
		 VALUES ($1, $2, $3, $4)
		 RETURNING snapshot_id, hiveshare_id, created_by, name, description, created_at`,
		hiveshareID, userID, name, description,
	).Scan(&snap.SnapshotID, &snap.HiveshareID, &snap.CreatedBy, &snap.Name, &snap.Description, &snap.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert snapshot: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO hiveshare_snapshot_entries
		     (snapshot_id, entry_id, content, summary, embedding, tags, metadata,
		      source_type, source_ref, source_url, tool)
		 SELECT $1, id, content, summary, embedding, tags, metadata,
		        source_type, source_ref, source_url, tool
		 FROM hives
		 WHERE hiveshare_id = $2`,
		snap.SnapshotID, hiveshareID,
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot entries: %w", err)
	}
	snap.EntryCount = int(tag.RowsAffected())

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *HistoryStore) ListSnapshots(ctx context.Context, hiveshareID uuid.UUID) ([]*models.Snapshot, error) {
	rows, err := s.db.Query(ctx,
		`SELECT s.snapshot_id, s.hiveshare_id, s.created_by, s.name, s.description, s.created_at,
		        (SELECT COUNT(*) FROM hiveshare_snapshot_entries se WHERE se.snapshot_id = s.snapshot_id) AS entry_count
		 FROM hiveshare_snapshots s
		 WHERE s.hiveshare_id = $1
		 ORDER BY s.created_at DESC`,
		hiveshareID,
	)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var result []*models.Snapshot
	for rows.Next() {
		var snap models.Snapshot
		if err := rows.Scan(&snap.SnapshotID, &snap.HiveshareID, &snap.CreatedBy, &snap.Name,
			&snap.Description, &snap.CreatedAt, &snap.EntryCount); err != nil {
			return nil, err
		}
		result = append(result, &snap)
	}
	return result, rows.Err()
}

func (s *HistoryStore) GetSnapshot(ctx context.Context, snapshotID int64, hiveshareID uuid.UUID) (*models.Snapshot, []*models.HistoryEntry, error) {
	var snap models.Snapshot
	err := s.db.QueryRow(ctx,
		`SELECT s.snapshot_id, s.hiveshare_id, s.created_by, s.name, s.description, s.created_at,
		        (SELECT COUNT(*) FROM hiveshare_snapshot_entries se WHERE se.snapshot_id = s.snapshot_id)
		 FROM hiveshare_snapshots s
		 WHERE s.snapshot_id = $1 AND s.hiveshare_id = $2`,
		snapshotID, hiveshareID,
	).Scan(&snap.SnapshotID, &snap.HiveshareID, &snap.CreatedBy, &snap.Name,
		&snap.Description, &snap.CreatedAt, &snap.EntryCount)
	if err != nil {
		return nil, nil, fmt.Errorf("get snapshot: %w", err)
	}

	rows, err := s.db.Query(ctx,
		`SELECT entry_id, content, summary, (embedding IS NOT NULL) AS has_embedding,
		        tags, metadata, source_type, source_ref, source_url, tool
		 FROM hiveshare_snapshot_entries
		 WHERE snapshot_id = $1`,
		snapshotID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("get snapshot entries: %w", err)
	}
	defer rows.Close()

	var entries []*models.HistoryEntry
	for rows.Next() {
		e := &models.HistoryEntry{HiveshareID: snap.HiveshareID}
		if err := rows.Scan(&e.EntryID, &e.Content, &e.Summary, &e.HasEmbedding,
			&e.Tags, &e.Metadata, &e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool); err != nil {
			return nil, nil, err
		}
		entries = append(entries, e)
	}
	return &snap, entries, rows.Err()
}

type RestoreResult struct {
	Hiveshare      *models.Hiveshare
	EntriesCreated int
	NullEmbeddings []uuid.UUID
}

func (s *HistoryStore) RestoreSnapshot(ctx context.Context, snapshotID int64, sourceHiveshareID, userID uuid.UUID, name string) (*RestoreResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var hs models.Hiveshare
	err = tx.QueryRow(ctx,
		`INSERT INTO hiveshares (name, description, owner_id)
		 SELECT $1, s.description, $2
		 FROM hiveshare_snapshots s
		 WHERE s.snapshot_id = $3 AND s.hiveshare_id = $4
		 RETURNING id, name, description, owner_id, settings, created_at, updated_at`,
		name, userID, snapshotID, sourceHiveshareID,
	).Scan(&hs.ID, &hs.Name, &hs.Description, &hs.OwnerID, &hs.Settings, &hs.CreatedAt, &hs.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create hiveshare from snapshot: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO hiveshare_members (hiveshare_id, user_id, role) VALUES ($1, $2, 'all')`,
		hs.ID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert owner member: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO hives
		     (hiveshare_id, user_id, source_type, source_ref, source_url,
		      tool, content, summary, embedding, tags, metadata)
		 SELECT $1, $2, source_type, source_ref, source_url,
		        tool, content, summary, embedding, tags, metadata
		 FROM hiveshare_snapshot_entries
		 WHERE snapshot_id = $3`,
		hs.ID, userID, snapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("restore snapshot entries: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT id FROM hives WHERE hiveshare_id = $1 AND embedding IS NULL`,
		hs.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("find null embeddings: %w", err)
	}
	defer rows.Close()
	var nullEmbeddings []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		nullEmbeddings = append(nullEmbeddings, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	hs.Role = models.RoleAll
	hs.MemberCount = 1
	return &RestoreResult{
		Hiveshare:      &hs,
		EntriesCreated: int(tag.RowsAffected()),
		NullEmbeddings: nullEmbeddings,
	}, nil
}

func (s *HistoryStore) DeleteSnapshot(ctx context.Context, snapshotID int64, hiveshareID uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM hiveshare_snapshots WHERE snapshot_id = $1 AND hiveshare_id = $2`,
		snapshotID, hiveshareID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ── Copy entries ─────────────────────────────────────────────────────────────

type CopyResult struct {
	Entry        *models.Hive
	HasEmbedding bool
}

func (s *HistoryStore) CopyEntries(ctx context.Context, targetHiveshareID, userID uuid.UUID, entryIDs []uuid.UUID) ([]*CopyResult, error) {
	if len(entryIDs) == 0 {
		return nil, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var results []*CopyResult
	for _, eid := range entryIDs {
		var e models.Hive
		var hasEmb bool
		var emb *pgvector.Vector
		var sourceHS uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT h.id, h.hiveshare_id, h.source_type, h.source_ref, h.source_url, h.tool,
			        h.content, h.summary, h.embedding, h.tags, h.metadata
			 FROM hives h
			 JOIN hiveshare_members m ON m.hiveshare_id = h.hiveshare_id AND m.user_id = $2
			 WHERE h.id = $1`,
			eid, userID,
		).Scan(&e.ID, &sourceHS, &e.SourceType, &e.SourceRef, &e.SourceURL, &e.Tool,
			&e.Content, &e.Summary, &emb, &e.Tags, &e.Metadata)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Distinguish missing vs forbidden without leaking existence across tenants.
				var exists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hives WHERE id = $1)`, eid).Scan(&exists)
				if exists {
					return nil, ErrForbidden
				}
				return nil, fmt.Errorf("read source entry %s: %w", eid, err)
			}
			return nil, fmt.Errorf("read source entry %s: %w", eid, err)
		}
		_ = sourceHS

		var embVal interface{}
		if emb != nil {
			embVal = emb
			hasEmb = true
		}

		baseRef := e.SourceRef
		var newEntry models.Hive
		for n := 0; ; n++ {
			ref := baseRef
			if n > 0 {
				ref = fmt.Sprintf("%s-%d", baseRef, n+1)
			}
			err = tx.QueryRow(ctx,
				`INSERT INTO hives
				     (hiveshare_id, user_id, source_type, source_ref, source_url,
				      tool, content, summary, embedding, tags, metadata)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				 RETURNING id, hiveshare_id, user_id, source_type, source_ref, source_url,
				           tool, content, summary, tags, metadata, views, reuses, created_at, updated_at`,
				targetHiveshareID, userID, e.SourceType, ref, e.SourceURL,
				e.Tool, e.Content, e.Summary, embVal, e.Tags, e.Metadata,
			).Scan(&newEntry.ID, &newEntry.HiveshareID, &newEntry.UserID, &newEntry.SourceType,
				&newEntry.SourceRef, &newEntry.SourceURL, &newEntry.Tool, &newEntry.Content,
				&newEntry.Summary, &newEntry.Tags, &newEntry.Metadata, &newEntry.Views,
				&newEntry.Reuses, &newEntry.CreatedAt, &newEntry.UpdatedAt)
			if err == nil {
				break
			}
			if n >= 9 || !isUniqueViolation(err) {
				return nil, fmt.Errorf("copy entry %s: %w", eid, err)
			}
		}

		results = append(results, &CopyResult{Entry: &newEntry, HasEmbedding: hasEmb})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return results, nil
}
