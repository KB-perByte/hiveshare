package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ServiceAccountStore manages service accounts used by CI agents and automated
// pipelines. Keys are prefixed hvsa_ and SHA-256 hashed at rest.
type ServiceAccountStore struct {
	db *pgxpool.Pool
}

func NewServiceAccountStore(db *pgxpool.Pool) *ServiceAccountStore {
	return &ServiceAccountStore{db: db}
}

// Create inserts a new service account and returns the cleartext key (shown once).
func (s *ServiceAccountStore) Create(ctx context.Context, hiveshareID, createdBy uuid.UUID, name, role string) (*models.ServiceAccount, error) {
	key, err := generateServiceAccountKey()
	if err != nil {
		return nil, err
	}
	var sa models.ServiceAccount
	err = s.db.QueryRow(ctx,
		`INSERT INTO service_accounts (hiveshare_id, name, key_hash, role, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, hiveshare_id, name, role, created_at`,
		hiveshareID, name, hashServiceAccountKey(key), role, createdBy,
	).Scan(&sa.ID, &sa.HiveshareID, &sa.Name, &sa.Role, &sa.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create service account: %w", err)
	}
	sa.Key = key
	return &sa, nil
}

// GetByKey looks up a service account by its cleartext key.
func (s *ServiceAccountStore) GetByKey(ctx context.Context, key string) (*models.ServiceAccount, error) {
	var sa models.ServiceAccount
	err := s.db.QueryRow(ctx,
		`SELECT id, hiveshare_id, name, role, created_at
		 FROM service_accounts WHERE key_hash = $1`,
		hashServiceAccountKey(key),
	).Scan(&sa.ID, &sa.HiveshareID, &sa.Name, &sa.Role, &sa.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get service account by key: %w", err)
	}
	return &sa, nil
}

// List returns all service accounts for a hiveshare.
func (s *ServiceAccountStore) List(ctx context.Context, hiveshareID uuid.UUID) ([]*models.ServiceAccount, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, hiveshare_id, name, role, created_at, last_used_at
		 FROM service_accounts WHERE hiveshare_id = $1 ORDER BY created_at`,
		hiveshareID,
	)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}
	defer rows.Close()
	var result []*models.ServiceAccount
	for rows.Next() {
		var sa models.ServiceAccount
		if err := rows.Scan(&sa.ID, &sa.HiveshareID, &sa.Name, &sa.Role, &sa.CreatedAt, &sa.LastUsedAt); err != nil {
			return nil, err
		}
		result = append(result, &sa)
	}
	return result, rows.Err()
}

// TouchLastUsed updates last_used_at asynchronously; errors are silently dropped.
func (s *ServiceAccountStore) TouchLastUsed(ctx context.Context, id uuid.UUID) {
	s.db.Exec(ctx, `UPDATE service_accounts SET last_used_at = NOW() WHERE id = $1`, id)
}

// Delete removes a service account. Returns false if not found.
func (s *ServiceAccountStore) Delete(ctx context.Context, id, hiveshareID uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM service_accounts WHERE id = $1 AND hiveshare_id = $2`, id, hiveshareID,
	)
	if err != nil {
		return false, fmt.Errorf("delete service account: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func generateServiceAccountKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "hvsa_" + hex.EncodeToString(b), nil
}

func hashServiceAccountKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// HashServiceAccountKey is exported for use in auth middleware.
func HashServiceAccountKey(key string) string {
	return hashServiceAccountKey(key)
}

