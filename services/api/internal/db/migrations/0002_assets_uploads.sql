-- 0002_assets_uploads: media assets + upload tracking (M3)

CREATE TABLE IF NOT EXISTS assets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sha256              TEXT NOT NULL,
    media_type          TEXT NOT NULL CHECK (media_type IN ('photo','video')),
    byte_size           BIGINT NOT NULL,
    width               INT,
    height              INT,
    duration_ms         INT,
    captured_at         TIMESTAMPTZ,
    exif                JSONB,
    live_photo_group_id UUID,
    storage_key         TEXT NOT NULL,
    thumb_key           TEXT,
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','uploading','uploaded','verified','complete','failed')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, sha256)
);

CREATE INDEX IF NOT EXISTS assets_user_captured_idx ON assets(user_id, captured_at DESC);

CREATE TABLE IF NOT EXISTS uploads (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id       UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    device_id      UUID REFERENCES devices(id) ON DELETE SET NULL,
    status         TEXT NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','uploading','uploaded','failed')),
    bytes_uploaded BIGINT NOT NULL DEFAULT 0,
    parts          JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS uploads_asset_idx ON uploads(asset_id);
