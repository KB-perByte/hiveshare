package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrForbidden is returned when the caller lacks membership on a source hiveshare.
var ErrForbidden = errors.New("forbidden")

// ErrSnapshotTooLarge is returned when a hiveshare has too many entries to snapshot.
var ErrSnapshotTooLarge = errors.New("snapshot too large")

// IsUniqueViolation reports whether err is a Postgres unique_violation (23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
