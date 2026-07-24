package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// MarkJob upserts a processing-job row for (asset, type, version).
func (s *Store) MarkJob(ctx context.Context, assetID, jobType, version, status, errMsg string) error {
	var started, completed *time.Time
	now := time.Now()
	switch status {
	case "processing":
		started = &now
	case "completed", "failed_permanent", "failed_retryable":
		completed = &now
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO asset_processing_jobs(asset_id, job_type, worker_version, status, error_message, started_at, completed_at, attempts)
		VALUES($1,$2,$3,$4,$5,$6,$7,1)
		ON CONFLICT (asset_id, job_type, worker_version) DO UPDATE SET
		  status=EXCLUDED.status,
		  error_message=EXCLUDED.error_message,
		  attempts=asset_processing_jobs.attempts+1,
		  started_at=COALESCE(asset_processing_jobs.started_at, EXCLUDED.started_at),
		  completed_at=EXCLUDED.completed_at,
		  updated_at=now()`,
		assetID, jobType, version, status, nullIf(errMsg), started, completed)
	return err
}

// SaveRaw stores the raw extracted metadata JSON.
func (s *Store) SaveRaw(ctx context.Context, assetID string, raw []byte, source, version string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO asset_metadata(asset_id, raw_metadata_json, metadata_source, metadata_worker_version, extracted_at)
		VALUES($1,$2,$3,$4,now())
		ON CONFLICT (asset_id) DO UPDATE SET
		  raw_metadata_json=EXCLUDED.raw_metadata_json,
		  metadata_source=EXCLUDED.metadata_source,
		  metadata_worker_version=EXCLUDED.metadata_worker_version,
		  extracted_at=now(), updated_at=now()`,
		assetID, raw, source, version)
	return err
}

// RawMetadata returns the stored raw JSON for normalization.
func (s *Store) RawMetadata(ctx context.Context, assetID string) ([]byte, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT raw_metadata_json FROM asset_metadata WHERE asset_id=$1`, assetID).Scan(&raw)
	return raw, err
}

type Technical struct {
	OriginalFilename                                     *string
	MimeType, ContainerFormat                            *string
	FileSize                                             *int64
	Width, Height, DurationMs, Orientation, ISO          *int
	FrameRate, Aperture, FocalLength                     *float64
	Bitrate                                              *int64
	VideoCodec, AudioCodec                               *string
	CameraMake, CameraModel, LensMake, LensModel         *string
	ExposureTime                                         *string
	CapturedAt                                           *time.Time
	CapturedAtSource, CapturedAtConfidence, CapturedTZ   *string
	Latitude, Longitude, Altitude                        *float64
}

func (s *Store) SaveTechnical(ctx context.Context, assetID string, t Technical) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO asset_technical_metadata(
		  asset_id, original_filename, mime_type, container_format, file_size, width, height,
		  duration_ms, orientation, frame_rate, bitrate, video_codec, audio_codec,
		  camera_make, camera_model, lens_make, lens_model, iso, aperture, exposure_time, focal_length,
		  captured_at, captured_at_source, captured_at_confidence, captured_timezone,
		  latitude, longitude, altitude)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
		ON CONFLICT (asset_id) DO UPDATE SET
		  original_filename=EXCLUDED.original_filename, mime_type=EXCLUDED.mime_type,
		  container_format=EXCLUDED.container_format, file_size=EXCLUDED.file_size,
		  width=EXCLUDED.width, height=EXCLUDED.height, duration_ms=EXCLUDED.duration_ms,
		  orientation=EXCLUDED.orientation, frame_rate=EXCLUDED.frame_rate, bitrate=EXCLUDED.bitrate,
		  video_codec=EXCLUDED.video_codec, audio_codec=EXCLUDED.audio_codec,
		  camera_make=EXCLUDED.camera_make, camera_model=EXCLUDED.camera_model,
		  lens_make=EXCLUDED.lens_make, lens_model=EXCLUDED.lens_model,
		  iso=EXCLUDED.iso, aperture=EXCLUDED.aperture, exposure_time=EXCLUDED.exposure_time,
		  focal_length=EXCLUDED.focal_length, captured_at=EXCLUDED.captured_at,
		  captured_at_source=EXCLUDED.captured_at_source, captured_at_confidence=EXCLUDED.captured_at_confidence,
		  captured_timezone=EXCLUDED.captured_timezone,
		  latitude=EXCLUDED.latitude, longitude=EXCLUDED.longitude, altitude=EXCLUDED.altitude,
		  updated_at=now()`,
		assetID, t.OriginalFilename, t.MimeType, t.ContainerFormat, t.FileSize, t.Width, t.Height,
		t.DurationMs, t.Orientation, t.FrameRate, t.Bitrate, t.VideoCodec, t.AudioCodec,
		t.CameraMake, t.CameraModel, t.LensMake, t.LensModel, t.ISO, t.Aperture, t.ExposureTime, t.FocalLength,
		t.CapturedAt, t.CapturedAtSource, t.CapturedAtConfidence, t.CapturedTZ,
		t.Latitude, t.Longitude, t.Altitude)
	return err
}

// SaveSearchDoc builds/updates the full-text search document (filename, date, camera).
func (s *Store) SaveSearchDoc(ctx context.Context, assetID, text, version string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO asset_search_documents(asset_id, searchable_text, search_vector, index_version, indexed_at)
		VALUES($1,$2, to_tsvector('simple',$2), $3, now())
		ON CONFLICT (asset_id) DO UPDATE SET
		  searchable_text=EXCLUDED.searchable_text,
		  search_vector=to_tsvector('simple', EXCLUDED.searchable_text),
		  index_version=EXCLUDED.index_version, indexed_at=now(), updated_at=now()`,
		assetID, text, version)
	return err
}

// TechnicalText returns filename + camera for the search document.
func (s *Store) TechnicalText(ctx context.Context, assetID string) (filename, camera string, capturedAt *time.Time, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(original_filename,''),
		       TRIM(COALESCE(camera_make,'') || ' ' || COALESCE(camera_model,'')),
		       captured_at
		FROM asset_technical_metadata WHERE asset_id=$1`, assetID).Scan(&filename, &camera, &capturedAt)
	return
}

func nullIf(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
