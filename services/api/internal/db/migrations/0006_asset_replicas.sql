-- 0006_asset_replicas: track replication of each asset to secondary targets (P2-5)

CREATE TABLE IF NOT EXISTS asset_replicas (
    asset_id      UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    target        TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','replicated','failed')),
    error         TEXT,
    replicated_at TIMESTAMPTZ,
    PRIMARY KEY (asset_id, target)
);

CREATE INDEX IF NOT EXISTS asset_replicas_status_idx ON asset_replicas(target, status);
