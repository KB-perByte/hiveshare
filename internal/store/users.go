package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KB-perByte/hiveshare/internal/models"
)

type UserStore struct {
	db *pgxpool.Pool
}

func NewUserStore(db *pgxpool.Pool) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, email, name string) (*models.User, error) {
	key, err := generateAPIKey()
	if err != nil {
		return nil, err
	}
	var u models.User
	err = s.db.QueryRow(ctx,
		`INSERT INTO users (email, name, api_key) VALUES ($1, $2, $3)
		 RETURNING id, email, name, api_key, created_at`,
		email, name, key,
	).Scan(&u.ID, &u.Email, &u.Name, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

func (s *UserStore) GetByAPIKey(ctx context.Context, key string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, api_key, created_at FROM users WHERE api_key = $1`, key,
	).Scan(&u.ID, &u.Email, &u.Name, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by api_key: %w", err)
	}
	return &u, nil
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, api_key, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.APIKey, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, email, name, api_key, created_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.APIKey, &u.CreatedAt)
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
