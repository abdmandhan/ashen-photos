package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type TechnicalMetadata struct {
	MimeType     *string    `json:"mime_type"`
	Width        *int       `json:"width"`
	Height       *int       `json:"height"`
	DurationMs   *int       `json:"duration_ms"`
	CameraMake   *string    `json:"camera_make"`
	CameraModel  *string    `json:"camera_model"`
	LensModel    *string    `json:"lens_model"`
	ISO          *int       `json:"iso"`
	Aperture     *float64   `json:"aperture"`
	FocalLength  *float64   `json:"focal_length"`
	CapturedAt   *time.Time `json:"captured_at"`
	Latitude     *float64   `json:"latitude"`
	Longitude    *float64   `json:"longitude"`
	Altitude     *float64   `json:"altitude"`
}

type ProcessingJob struct {
	JobType     string     `json:"job_type"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	Error       *string    `json:"error_message,omitempty"`
	CompletedAt *time.Time `json:"completed_at"`
}

// AssetTechnical returns normalized technical metadata for an owned asset.
func (s *Store) AssetTechnical(ctx context.Context, userID, assetID string) (TechnicalMetadata, error) {
	var t TechnicalMetadata
	err := s.pool.QueryRow(ctx, `
		SELECT tm.mime_type, tm.width, tm.height, tm.duration_ms,
		       tm.camera_make, tm.camera_model, tm.lens_model, tm.iso, tm.aperture, tm.focal_length,
		       tm.captured_at, tm.latitude, tm.longitude, tm.altitude
		FROM asset_technical_metadata tm
		JOIN assets a ON a.id = tm.asset_id
		WHERE tm.asset_id=$1 AND a.user_id=$2`, assetID, userID,
	).Scan(&t.MimeType, &t.Width, &t.Height, &t.DurationMs,
		&t.CameraMake, &t.CameraModel, &t.LensModel, &t.ISO, &t.Aperture, &t.FocalLength,
		&t.CapturedAt, &t.Latitude, &t.Longitude, &t.Altitude)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ProcessingJobs returns the metadata jobs for an owned asset.
func (s *Store) ProcessingJobs(ctx context.Context, userID, assetID string) ([]ProcessingJob, error) {
	// Ownership check first.
	var owned bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets WHERE id=$1 AND user_id=$2)`, assetID, userID,
	).Scan(&owned); err != nil {
		return nil, err
	}
	if !owned {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT job_type, status, attempts, error_message, completed_at
		FROM asset_processing_jobs WHERE asset_id=$1 ORDER BY created_at`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProcessingJob
	for rows.Next() {
		var j ProcessingJob
		if err := rows.Scan(&j.JobType, &j.Status, &j.Attempts, &j.Error, &j.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// DeriveMetadataStatus summarizes job rows into an asset-level status (FR-015).
func DeriveMetadataStatus(jobs []ProcessingJob) string {
	if len(jobs) == 0 {
		return "NOT_STARTED"
	}
	core := map[string]bool{"metadata:extract": false, "metadata:normalize": false, "metadata:index": false}
	anyProcessing, anyFailed := false, false
	for _, j := range jobs {
		switch j.Status {
		case "completed":
			if _, ok := core[j.JobType]; ok {
				core[j.JobType] = true
			}
		case "processing", "pending":
			anyProcessing = true
		case "failed_permanent", "failed_retryable":
			anyFailed = true
		}
	}
	allCore := core["metadata:extract"] && core["metadata:normalize"] && core["metadata:index"]
	switch {
	case allCore:
		return "COMPLETED"
	case anyFailed:
		return "FAILED"
	case anyProcessing:
		return "PROCESSING"
	default:
		return "PENDING"
	}
}
