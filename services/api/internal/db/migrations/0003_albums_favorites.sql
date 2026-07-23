-- 0003_albums_favorites: albums, album membership, favorites (P2-1)

CREATE TABLE IF NOT EXISTS albums (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    cover_asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS albums_user_idx ON albums(user_id);

CREATE TABLE IF NOT EXISTS album_assets (
    album_id UUID NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (album_id, asset_id)
);

CREATE INDEX IF NOT EXISTS album_assets_asset_idx ON album_assets(asset_id);

ALTER TABLE assets ADD COLUMN IF NOT EXISTS favorite BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS assets_favorite_idx ON assets(user_id) WHERE favorite;
