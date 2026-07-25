package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KB-perByte/hiveshare/internal/models"
)

// UserStore manages user accounts. API keys are stored as SHA-256 hashes;
// the cleartext key is returned only once at registration time.
type UserStore struct {
	db *pgxpool.Pool
}

// NewUserStore returns a UserStore backed by the given connection pool.
func NewUserStore(db *pgxpool.Pool) *UserStore {
	return &UserStore{db: db}
}

// Create inserts a user, storing SHA-256(api_key) in the database.
// The returned User.APIKey is the cleartext key and is only available once.
func (s *UserStore) Create(ctx context.Context, email, name string) (*models.User, error) {
	key, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	var u models.User
	err = s.db.QueryRow(ctx,
		`INSERT INTO users (email, name, api_key) VALUES ($1, $2, $3)
		 RETURNING id, email, name, created_at`,
		email, name, hashAPIKey(key),
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	u.APIKey = key // cleartext returned once at registration
	return &u, nil
}

// GetByAPIKey looks up a user by their cleartext API key (hashed before querying).
func (s *UserStore) GetByAPIKey(ctx context.Context, key string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, created_at FROM users WHERE api_key = $1`, hashAPIKey(key),
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by api_key: %w", err)
	}
	return &u, nil
}

// GetByEmail returns the user with the given email address.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// GetByID returns the user with the given UUID.
func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, created_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "hvs_" + hex.EncodeToString(b), nil
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// HashAPIKey returns the SHA-256 hex digest used for storage and cache keys.
func HashAPIKey(key string) string {
	return hashAPIKey(key)
}
