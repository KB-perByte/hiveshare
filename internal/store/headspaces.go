package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KB-perByte/hiveshare/internal/models"
)

type HiveshareStore struct {
	db *pgxpool.Pool
}

func NewHiveshareStore(db *pgxpool.Pool) *HiveshareStore {
	return &HiveshareStore{db: db}
}

func (s *HiveshareStore) Create(ctx context.Context, name, description string, ownerID uuid.UUID) (*models.Hiveshare, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var hs models.Hiveshare
	err = tx.QueryRow(ctx,
		`INSERT INTO hiveshares (name, description, owner_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, description, owner_id, settings, created_at, updated_at`,
		name, description, ownerID,
	).Scan(&hs.ID, &hs.Name, &hs.Description, &hs.OwnerID, &hs.Settings, &hs.CreatedAt, &hs.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert hiveshare: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO hiveshare_members (hiveshare_id, user_id, role) VALUES ($1, $2, 'owner')`,
		hs.ID, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert owner member: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	hs.Role = "owner"
	hs.MemberCount = 1
	return &hs, nil
}

func (s *HiveshareStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]*models.Hiveshare, error) {
	rows, err := s.db.Query(ctx,
		`SELECT h.id, h.name, h.description, h.owner_id, h.settings, h.created_at, h.updated_at,
		        hm.role,
		        (SELECT COUNT(*) FROM hiveshare_members hm2 WHERE hm2.hiveshare_id = h.id) AS member_count
		 FROM hiveshares h
		 JOIN hiveshare_members hm ON hm.hiveshare_id = h.id AND hm.user_id = $1
		 ORDER BY h.created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Hiveshare
	for rows.Next() {
		var hs models.Hiveshare
		if err := rows.Scan(&hs.ID, &hs.Name, &hs.Description, &hs.OwnerID, &hs.Settings,
			&hs.CreatedAt, &hs.UpdatedAt, &hs.Role, &hs.MemberCount); err != nil {
			return nil, err
		}
		result = append(result, &hs)
	}
	return result, rows.Err()
}

func (s *HiveshareStore) Get(ctx context.Context, id, userID uuid.UUID) (*models.Hiveshare, error) {
	var hs models.Hiveshare
	err := s.db.QueryRow(ctx,
		`SELECT h.id, h.name, h.description, h.owner_id, h.settings, h.created_at, h.updated_at,
		        hm.role,
		        (SELECT COUNT(*) FROM hiveshare_members hm2 WHERE hm2.hiveshare_id = h.id) AS member_count
		 FROM hiveshares h
		 JOIN hiveshare_members hm ON hm.hiveshare_id = h.id AND hm.user_id = $2
		 WHERE h.id = $1`, id, userID,
	).Scan(&hs.ID, &hs.Name, &hs.Description, &hs.OwnerID, &hs.Settings,
		&hs.CreatedAt, &hs.UpdatedAt, &hs.Role, &hs.MemberCount)
	if err != nil {
		return nil, fmt.Errorf("get hiveshare: %w", err)
	}
	return &hs, nil
}

func (s *HiveshareStore) Update(ctx context.Context, id uuid.UUID, name, description string) (*models.Hiveshare, error) {
	var hs models.Hiveshare
	err := s.db.QueryRow(ctx,
		`UPDATE hiveshares SET name = $2, description = $3, updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, name, description, owner_id, settings, created_at, updated_at`,
		id, name, description,
	).Scan(&hs.ID, &hs.Name, &hs.Description, &hs.OwnerID, &hs.Settings, &hs.CreatedAt, &hs.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update hiveshare: %w", err)
	}
	return &hs, nil
}

func (s *HiveshareStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM hiveshares WHERE id = $1`, id)
	return err
}

func (s *HiveshareStore) IsMember(ctx context.Context, hiveshareID, userID uuid.UUID) (string, error) {
	var role string
	err := s.db.QueryRow(ctx,
		`SELECT role FROM hiveshare_members WHERE hiveshare_id = $1 AND user_id = $2`,
		hiveshareID, userID,
	).Scan(&role)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return role, err
}

func (s *HiveshareStore) ListMembers(ctx context.Context, hiveshareID uuid.UUID) ([]*models.Member, error) {
	rows, err := s.db.Query(ctx,
		`SELECT hm.hiveshare_id, hm.user_id, u.name, u.email, hm.role, hm.invited_by, hm.joined_at
		 FROM hiveshare_members hm
		 JOIN users u ON u.id = hm.user_id
		 WHERE hm.hiveshare_id = $1
		 ORDER BY hm.joined_at`, hiveshareID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.HiveshareID, &m.UserID, &m.Name, &m.Email, &m.Role, &m.InvitedBy, &m.JoinedAt); err != nil {
			return nil, err
		}
		result = append(result, &m)
	}
	return result, rows.Err()
}

func (s *HiveshareStore) RemoveMember(ctx context.Context, hiveshareID, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM hiveshare_members WHERE hiveshare_id = $1 AND user_id = $2 AND role != 'owner'`,
		hiveshareID, userID,
	)
	return err
}

func (s *HiveshareStore) AddMember(ctx context.Context, hiveshareID, userID, invitedBy uuid.UUID, role string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO hiveshare_members (hiveshare_id, user_id, role, invited_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (hiveshare_id, user_id) DO NOTHING`,
		hiveshareID, userID, role, invitedBy,
	)
	return err
}

// CreateInvitation creates a pending invitation.
func (s *HiveshareStore) CreateInvitation(ctx context.Context, inv *models.Invitation) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO invitations (hiveshare_id, email, invited_by, token, role)
		 VALUES ($1, $2, $3, $4, $5)`,
		inv.HiveshareID, inv.Email, inv.InvitedBy, inv.Token, inv.Role,
	)
	return err
}

func (s *HiveshareStore) GetInvitation(ctx context.Context, token string) (*models.Invitation, error) {
	var inv models.Invitation
	err := s.db.QueryRow(ctx,
		`SELECT i.id, i.hiveshare_id, i.email, i.invited_by, i.token, i.role, i.status, i.created_at, i.expires_at,
		        h.name
		 FROM invitations i
		 JOIN hiveshares h ON h.id = i.hiveshare_id
		 WHERE i.token = $1 AND i.status = 'pending' AND i.expires_at > NOW()`, token,
	).Scan(&inv.ID, &inv.HiveshareID, &inv.Email, &inv.InvitedBy, &inv.Token,
		&inv.Role, &inv.Status, &inv.CreatedAt, &inv.ExpiresAt, &inv.HiveShareName)
	if err != nil {
		return nil, fmt.Errorf("get invitation: %w", err)
	}
	return &inv, nil
}

func (s *HiveshareStore) AcceptInvitation(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE invitations SET status = 'accepted' WHERE token = $1`, token,
	)
	return err
}
