-- 0005_duplicates: perceptual hash, near-dup grouping, soft delete (P2-4)

ALTER TABLE assets ADD COLUMN IF NOT EXISTS phash        BIGINT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS dup_group_id UUID;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS deleted_at   TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS assets_dup_group_idx ON assets(dup_group_id) WHERE dup_group_id IS NOT NULL;
-- candidate window for cheap near-dup comparison: same user + dimensions
CREATE INDEX IF NOT EXISTS assets_phash_window_idx ON assets(user_id, width, height) WHERE phash IS NOT NULL;
