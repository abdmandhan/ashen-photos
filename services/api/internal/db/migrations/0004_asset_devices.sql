-- 0004_asset_devices: which devices have seen/hold each asset (P2-3)

CREATE TABLE IF NOT EXISTS asset_devices (
    asset_id  UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (asset_id, device_id)
);

CREATE INDEX IF NOT EXISTS asset_devices_device_idx ON asset_devices(device_id);
