-- 0007_metadata: metadata worker — jobs, raw+normalized metadata, technical fields, search doc (Phase 2b M1)

CREATE TABLE IF NOT EXISTS asset_processing_jobs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id       UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    job_type       TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    worker_version TEXT NOT NULL DEFAULT '1',
    attempts       INT  NOT NULL DEFAULT 0,
    error_code     TEXT,
    error_message  TEXT,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, job_type, worker_version)   -- BR-006: no duplicate active jobs
);

CREATE INDEX IF NOT EXISTS asset_jobs_asset_idx  ON asset_processing_jobs(asset_id);
CREATE INDEX IF NOT EXISTS asset_jobs_status_idx ON asset_processing_jobs(status);

CREATE TABLE IF NOT EXISTS asset_metadata (
    asset_id                 UUID PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
    raw_metadata_json        JSONB,
    normalized_metadata_json JSONB,
    metadata_source          TEXT,
    metadata_worker_version  TEXT,
    extracted_at             TIMESTAMPTZ,
    normalized_at            TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS asset_technical_metadata (
    asset_id               UUID PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
    original_filename      TEXT,
    mime_type              TEXT,
    container_format       TEXT,
    file_size              BIGINT,
    width                  INT,
    height                 INT,
    duration_ms            INT,
    orientation            INT,
    frame_rate             REAL,
    bitrate                BIGINT,
    video_codec            TEXT,
    audio_codec            TEXT,
    camera_make            TEXT,
    camera_model           TEXT,
    lens_make              TEXT,
    lens_model             TEXT,
    iso                    INT,
    aperture               REAL,
    exposure_time          TEXT,
    focal_length           REAL,
    captured_at            TIMESTAMPTZ,
    captured_at_source     TEXT,
    captured_at_confidence TEXT,
    captured_timezone      TEXT,
    latitude               DOUBLE PRECISION,
    longitude              DOUBLE PRECISION,
    altitude               DOUBLE PRECISION,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tech_meta_camera_idx ON asset_technical_metadata(camera_model);

CREATE TABLE IF NOT EXISTS asset_search_documents (
    asset_id              UUID PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
    searchable_text       TEXT,
    search_vector         TSVECTOR,
    indexed_metadata_json JSONB,
    index_version         TEXT,
    indexed_at            TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS asset_search_vector_idx ON asset_search_documents USING GIN(search_vector);
