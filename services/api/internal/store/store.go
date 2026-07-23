package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type Device struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Name          string     `json:"name"`
	Platform      string     `json:"platform"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	UploadedCount int64      `json:"uploaded_count"`
}

func (s *Store) CreateUser(ctx context.Context, email, hash string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users(email, password_hash) VALUES($1,$2)
		 RETURNING id, email, password_hash, created_at`,
		email, hash,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, created_at FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateDevice(ctx context.Context, userID, name, platform string) (Device, error) {
	var d Device
	err := s.pool.QueryRow(ctx,
		`INSERT INTO devices(user_id, name, platform) VALUES($1,$2,$3)
		 RETURNING id, user_id, name, platform, created_at, last_seen_at`,
		userID, name, platform,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeenAt)
	return d, err
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.user_id, d.name, d.platform, d.created_at, d.last_seen_at,
		       COUNT(ad.asset_id) AS uploaded_count
		FROM devices d
		LEFT JOIN asset_devices ad ON ad.device_id = d.id
		WHERE d.user_id=$1
		GROUP BY d.id
		ORDER BY d.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeenAt, &d.UploadedCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RenameDevice updates a device's display name (scoped to the user).
func (s *Store) RenameDevice(ctx context.Context, userID, id, name string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE devices SET name=$3 WHERE id=$1 AND user_id=$2`, id, userID, name)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchDevice bumps last_seen_at. Best-effort; no error if the device isn't the user's.
func (s *Store) TouchDevice(ctx context.Context, userID, id string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE devices SET last_seen_at=now() WHERE id=$1 AND user_id=$2`, id, userID)
}

// RecordAssetDevice notes that a device holds/saw an asset (dedup reconciliation).
// Idempotent; refreshes seen_at on repeat.
func (s *Store) RecordAssetDevice(ctx context.Context, assetID, deviceID string) {
	_, _ = s.pool.Exec(ctx,
		`INSERT INTO asset_devices(asset_id, device_id) VALUES($1,$2)
		 ON CONFLICT (asset_id, device_id) DO UPDATE SET seen_at=now()`, assetID, deviceID)
}
