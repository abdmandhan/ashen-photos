-- 0007_assets_created_idx: index for "latest backed up" timeline sort (created_at DESC)

CREATE INDEX IF NOT EXISTS assets_user_created_idx ON assets(user_id, created_at DESC);
