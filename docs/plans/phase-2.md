# Phase 2 — Better Photo Experience

> Goal: turn the reliable backup from Phase 1 into a real photo library — organize (albums, favorites), find (metadata search), trust (duplicate detection, multi-device), and protect (off-site replication).

> **Status: 🚧 IN PROGRESS** — P2-1 (albums+favorites) and P2-2 (search) done + verified. Web UI (P2-6) has albums, favorites, and filters live. Remaining: P2-3 multi-device, P2-4 duplicates, P2-5 replication, iOS UI.

**Progress:**
- ✅ **P2-1 Albums+Favorites** — migration `0003`, full CRUD + membership + favorite toggle. Curl-verified E2E.
- ✅ **P2-2 Search** — filterable `GET /assets` (from/to/media_type/device_id/favorite/album_id) + `GET /search/facets`. Verified.
- ✅ **P2-3 Multi-device** — `asset_devices` join (migration `0004`), `last_seen_at` via `X-Device-Id` header, per-device `uploaded_count` in `GET /devices`, `PATCH /devices/:id` rename, dedup reconciliation (device B seeing an existing asset is recorded without re-upload). Verified E2E.
- ✅ **P2-4 Duplicates** — worker computes dHash (64-bit), groups same-dimension assets within Hamming ≤10 into a `dup_group_id` (migration `0005`: `phash`, `dup_group_id`, `deleted_at`). `GET /duplicates`, `POST /assets/:id/resolve-duplicate` (delete=soft-delete / keep=dismiss). Soft-deleted assets excluded from timeline/stats/facets/dedup. Verified E2E on an isolated queue.
- 🚧 **P2-6 Web** — albums row, favorite hearts, filter chips (All/Photos/Videos/Favorites) live + browser-verified. iOS UI + duplicates/replication views pending.
- ⬜ P2-5 replication — not started.

**Infra note:** added `ASHEN_QUEUE_KEY` env override (API + worker) so a test pipeline can run on a private Redis list without the production worker stealing jobs.

---

## Success Criteria

Phase 2 is done when:

- A user can create albums, add/remove assets, and browse album contents on web + iOS.
- A user can favorite/unfavorite an asset; favorites are a filterable view everywhere.
- A user can search their library by date range, media type, device, favorite, and album — results paginate.
- The same photo backed up from two devices is stored once and shown once (already deduped by sha256; now surfaced and reconciled per-device).
- Multiple devices back up to one account concurrently without collisions; each device's status is visible.
- "Similar/duplicate" assets (same photo re-encoded, screenshots of screenshots) are flagged for review beyond exact-hash dedup.
- Every completed asset is replicated to a second storage target (S3/R2/another MinIO); replication status is observable and re-drivable.

---

## Non-Goals (defer to Phase 3)

AI/semantic search, OCR, image embeddings, face recognition, natural-language queries, smart collections, video transcoding, sharing/public albums, end-to-end encryption.

Phase 2 stays **single-user** (multi-device, not multi-user). Multi-user auth/isolation is a later phase.

---

## What Phase 1 Already Gives Us

- `assets` deduped by `(user_id, sha256)` — exact-duplicate storage already prevented.
- `devices` table + per-upload `device_id`.
- `live_photo_group_id` linkage.
- Worker pipeline (verify → thumbnail → EXIF → complete) with a Redis queue we can add job types to.
- Presigned storage behind a `storage` interface (swap/duplicate targets easily).
- Timeline (`GET /assets` keyset pagination), `GET /stats`.

Phase 2 extends these rather than replacing them.

---

## Workstreams

### 1. Albums & Favorites (API + DB)

**Schema (migration `0003_albums_favorites.sql`):**

```
albums        (id, user_id, name, cover_asset_id, created_at, updated_at)
album_assets  (album_id, asset_id, added_at, PRIMARY KEY(album_id, asset_id))
```

Favorites: add `assets.favorite BOOLEAN NOT NULL DEFAULT false` (+ partial index `WHERE favorite`).

**Endpoints:**

- [ ] `POST /albums` / `GET /albums` / `PATCH /albums/:id` / `DELETE /albums/:id`.
- [ ] `POST /albums/:id/assets` (add), `DELETE /albums/:id/assets/:assetId` (remove).
- [ ] `GET /albums/:id/assets` — paginated, presigned thumbs (reuse timeline shape).
- [ ] `PUT /assets/:id/favorite` (toggle) — body `{favorite: bool}`.
- [ ] Album cover: auto-pick first asset, overridable via `cover_asset_id`.

### 2. Search / Filtering (API)

Metadata search only (no AI). Extend `GET /assets` into a filterable query.

- [ ] `GET /assets?from=&to=&media_type=&device_id=&favorite=&album_id=&limit=&cursor=`.
- [ ] Keyset pagination stays `(captured_at, id)`; filters are `AND`ed in SQL.
- [ ] Indexes: `assets(user_id, media_type)`, `assets(user_id, favorite)`, existing `(user_id, captured_at)`.
- [ ] `GET /search/facets` — counts per media_type / device / favorite for filter UI.

> Full-text/semantic search is Phase 3. Phase 2 search = structured metadata filters.

### 3. Duplicate Detection (Worker)

Exact dupes already handled by sha256. Phase 2 adds **near-duplicate** flagging.

**Schema:** add `assets.phash BIGINT` (perceptual hash) + `assets.dup_group_id UUID`.

- [ ] Worker computes a perceptual hash (dHash/aHash, 64-bit) during thumbnail step; store `phash`.
- [ ] New `dedup` job (or piggyback verify): compare `phash` Hamming distance against the user's existing assets; within threshold → assign shared `dup_group_id`.
- [ ] `GET /duplicates` — groups of near-dupes for user review.
- [ ] `POST /assets/:id/resolve-duplicate` — keep/delete decision (soft-delete losers).

> Keep it cheap: compare only within a candidate window (same rough capture date / dimensions bucket), not全-library O(n²).

### 4. Multi-Device (API + iOS)

Concurrency already works (uploads keyed by sha256 + per-device rows). Phase 2 makes it first-class.

- [ ] `PATCH /devices/:id` — rename; `devices.last_seen_at` updated on any authed request from that device (middleware sets it via a device header).
- [ ] Client sends `X-Device-Id` header; middleware bumps `last_seen_at`.
- [ ] Per-device upload counters: `GET /devices` returns `{uploaded_count, last_seen_at}`.
- [ ] iOS: surface "this device" vs others; show other devices' activity.
- [ ] Reconcile: an asset uploaded by device A then re-seen by device B records B in a join without re-storing bytes (dedup path already returns `exists`; log the device association).

**Schema:** `asset_devices (asset_id, device_id, seen_at, PRIMARY KEY(asset_id, device_id))` — which devices have this asset locally.

### 5. External Replication (Worker + Infra)

Second storage target for durability (off the single MinIO).

**Schema:** `asset_replicas (asset_id, target, status, replicated_at, PRIMARY KEY(asset_id, target))` where `target` ∈ {`s3`, `r2`, `minio2`}. `status` ∈ {`pending`, `replicated`, `failed`}.

- [ ] Add a `replicate` job enqueued after an asset reaches `complete`.
- [ ] Worker streams object from primary → secondary target (server-side copy where possible), verifies size/hash, marks `replicated`.
- [ ] Second `storage` implementation config (endpoint/keys/bucket for target 2).
- [ ] `GET /replication/status` — counts pending/replicated/failed; `POST /replication/redrive` — requeue failed.
- [ ] Idempotent: skip if already `replicated`; safe to re-run.

### 6. Web + iOS UI

**Web (`apps/web/`):**

- [ ] Albums view: grid of albums → album detail (asset grid).
- [ ] Favorite toggle on tiles + favorites filter.
- [ ] Filter bar: date range, media type, device, favorite, album.
- [ ] Duplicates review screen (groups, keep/delete).
- [ ] Devices page: last-seen + per-device counts.
- [ ] Replication status widget.

**iOS (`apps/ios/`):**

- [ ] Favorite an asset (writes through to API).
- [ ] Albums (view + add current backup to album).
- [ ] Device identity + other-device visibility.
- [ ] Settings: replication opt-in indicator (read-only status).

---

## Data Model Delta (summary)

```
albums          (id, user_id, name, cover_asset_id, created_at, updated_at)
album_assets    (album_id, asset_id, added_at)
asset_devices   (asset_id, device_id, seen_at)
asset_replicas  (asset_id, target, status, replicated_at)

assets   += favorite BOOLEAN, phash BIGINT, dup_group_id UUID, deleted_at TIMESTAMPTZ (soft delete)
devices  += (uses existing last_seen_at; add uploaded_count via view or counter)
```

New status values stay within existing `assets.status`; soft-delete via `deleted_at` (timeline filters it out).

---

## Milestones

| #   | Milestone        | Deliverable                                                        |
| --- | ---------------- | ------------------------------------------------------------------ |
| P2-1 | Albums+Favorites | CRUD albums, add/remove assets, favorite toggle; curl-verified.    | ✅ done |
| P2-2 | Search           | Filterable `GET /assets` + facets; keyset pagination holds.        | ✅ done |
| P2-3 | Multi-device     | `last_seen_at`, per-device counts, `asset_devices` reconciliation. | ✅ done |
| P2-4 | Duplicates       | `phash` + near-dup grouping + review/resolve endpoints.            | ✅ done |
| P2-5 | Replication      | second target, `replicate` job, status + redrive.                 | ⬜ todo |
| P2-6 | UI               | Web: albums, favorites, filters ✅. iOS + dup/repl views pending.   | 🚧 partial |

P2-1 and P2-2 are independent and can go first (pure API). P2-3/P2-4/P2-5 each touch the worker or middleware. P2-6 lands after its backing endpoints are stable.

---

## Open Questions

- Perceptual hash algorithm + Hamming threshold? (Propose: dHash 64-bit, distance ≤ 10, tune on real data.)
- Duplicate comparison scope to avoid O(n²)? (Propose: bucket by capture-day + dimension class; compare within bucket.)
- Replication target for first cut — Cloudflare R2, AWS S3, or a second MinIO? (Propose: second MinIO on another host for a self-hosted story; R2 as a documented option.)
- Soft-delete vs hard-delete on duplicate resolution? (Propose: soft-delete `deleted_at`, purge job later.)
- Do we replicate thumbnails too, or regenerate on restore? (Propose: replicate originals only; thumbs are derived + cheap to rebuild.)

---

## First Concrete Steps

1. Migration `0003` — albums, album_assets, `assets.favorite`, indexes.
2. Album + favorite endpoints (P2-1), curl-verified against `nuc.test`.
3. Extend `GET /assets` with filters + facets (P2-2).
4. Then branch into worker tracks (dedup, replication) and UI.
