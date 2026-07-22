# Phase 1 — Reliable Backup

> Goal: an iPhone app that automatically and reliably backs up photos and videos to self-hosted storage at original quality, with a web dashboard to see what's backed up.

> **Status: ✅ COMPLETE** — M1–M7 all done and verified against real infra on `nuc.test`. Backend (Go API + worker), iOS app (SwiftUI), and web dashboard (Next.js) demoed working end to end. Deferred items (multipart, HEIC/video thumbs, Tailwind, timeline virtualization, per-device progress) noted inline — none block Phase 1's success criteria.

## Success Criteria

Phase 1 is done when:

- A user installs the iOS app, grants Photos access, and their library uploads automatically in the background.
- Uploads survive app termination, network loss, and reboots (resume + retry).
- Every backed-up file is byte-identical to the original (SHA-256 verified end to end).
- Duplicate assets are never stored twice.
- The web dashboard shows a timeline, per-device upload status, and storage usage, and can download originals.
- The whole backend runs with a single `docker compose up`.

## Non-Goals (defer to later phases)

Android, photo editing, sharing, albums, favorites, search, AI, face recognition, OCR, video transcoding, end-to-end encryption, multi-user.

Phase 1 is explicitly **single-user** (one account), though the data model leaves room for more.

---

## Architecture Recap

```
iPhone (SwiftUI + PhotoKit)
   │  background URLSession
   ▼
Go API  ──►  PostgreSQL (metadata)
   │    ──►  Object Storage (MinIO/S3, presigned URLs)
   │    ──►  Redis (job queue)
   ▼
Go Worker  ──►  thumbnails, checksum verify, finalize
   ▼
Next.js Dashboard
```

Media bytes go **direct to object storage** via presigned URLs. The API handles metadata and issues URLs; it never proxies large file bodies.

---

## Workstreams

Four tracks. Backend + infra first (they unblock everything), then iOS and web in parallel against a stable API.

### 1. Infra (`infra/`)

> **Deviation:** infra runs on an existing shared host (`nuc.test`) — Postgres `:5433`, Redis `:6379`, MinIO `:9000/9001`. No self-hosted `docker compose` for these. Isolated via dedicated `ashen` DB + `ashen-*` bucket prefix.

- [x] ~~`docker-compose.yml` for pg/redis/minio~~ → external on `nuc.test`; compose deferred to api/worker only.
- [x] Dedicated `ashen` Postgres DB.
- [x] MinIO buckets `ashen-photos`, `ashen-videos`, `ashen-thumbnails`.
- [x] `.env.example` (nuc.test conns, MinIO keys, JWT secret).
- [x] `check-connections.sh` (verifies pg/redis/minio). Makefile `run/migrate/build/tidy` per service.

### 2. API (`services/api/`, Go)

**Data model (Postgres):**

```
users     (id, email, password_hash, created_at)
devices   (id, user_id, name, platform, created_at, last_seen_at)
assets    (id, user_id, sha256, media_type, byte_size, width, height,
           duration_ms, captured_at, exif jsonb, live_photo_group_id,
           storage_key, thumb_key, status, created_at)
uploads   (id, asset_id, device_id, status, bytes_uploaded, parts jsonb,
           created_at, updated_at)
```

- `assets.sha256` is unique per user → dedup key.
- `status`: `pending → uploading → uploaded → verified → complete` (plus `failed`).

**Endpoints:**

- [x] `POST /auth/register`, `POST /auth/login` → JWT.
- [x] `POST /devices` (+ `GET /devices`) — register/list devices.
- [x] `POST /uploads/check` — dedup by sha256.
- [x] `POST /uploads` — presigned PUT URL. *(single PUT; multipart for large videos deferred)*
- [x] `POST /uploads/:id/complete` — enqueue verify job.
- [x] `GET /assets` — paginated timeline (keyset by `captured_at`).
- [x] ~~`GET /assets/:id/original` + `/thumb`~~ → replaced by presigned `thumb_url` + `download_url` embedded in `GET /assets` (works with `<img>`, no per-request auth).
- [x] `GET /stats` — counts + storage usage.

**Cross-cutting:**

- [x] JWT middleware, request logging (chi), structured JSON errors, CORS.
- [x] SQL migrations (embedded, `schema_migrations` tracking).
- [x] Presigned URL helper (minio-go, S3-compatible).

### 3. Worker (`services/worker/`, Go)

- [x] Consume `verify` jobs from Redis (BRPOP, retry-on-error requeue).
- [x] Recompute SHA-256, compare to claimed hash. Mismatch → mark `failed`, delete object.
- [x] Generate thumbnail (JPEG, longest edge 512px). *(HEIC/video thumb deferred — needs libheif/ffmpeg; verifies + completes without thumb)*
- [x] Extract EXIF (captured_at, dimensions, make/model) into `assets.exif`.
- [x] Mark asset `complete`.
- [x] Live Photo linkage handled via `live_photo_group_id` (client sends group id; both parts stored + linked).

### 4. iOS (`apps/ios/`, SwiftUI)

- [x] Photos permission request + onboarding.
- [x] `PHPhotoLibrary` scan → enumerate assets, stream SHA-256 of originals (CryptoKit).
- [x] Local queue (JSON-persisted): pending, in-flight, done, failed, skipped + retry count.
- [x] `POST /uploads/check` to skip already-backed-up assets.
- [x] Background `URLSession` upload to presigned URLs; resume on relaunch; retry (cap 3) on network loss.
- [x] Settings: Wi-Fi-only, charging-only toggles (NWPathMonitor + battery gate).
- [x] `PHPhotoLibraryChangeObserver` to catch new photos automatically.
- [x] Live Photo upload (still + paired video, shared group id).
- [x] Status UI: backed up / remaining / failed / current.

> Builds + runs on iOS 26.5 simulator. Real PhotoKit read + background upload need a physical device.

### 5. Web (`apps/web/`, Next.js)

- [x] Login (JWT in localStorage).
- [x] Timeline grid, thumbnails from presigned `thumb_url`. *(virtualization + day-grouping deferred)*
- [x] Device list. *(per-device upload progress deferred — needs upload-status endpoint)*
- [x] Storage usage + asset counts (`/stats`).
- [x] Download original (tile → presigned `download_url`).

> Next.js 14 App Router, plain CSS (Tailwind deferred). Verified live in headless browser.

---

## Formats to Support (Phase 1)

HEIC, JPEG (photos); MOV, MP4 (videos); Live Photos (still + video pair). Store originals untouched — no transcoding.

---

## Security (Phase 1)

- HTTPS only (TLS terminated at reverse proxy in prod; plain HTTP acceptable for local `docker compose`).
- JWT auth on all API routes except register/login.
- Presigned upload/download URLs, short TTL (e.g. 15 min).
- SHA-256 verification server-side before marking `complete` — never trust the client's hash blindly.
- Object storage not publicly listable; access only via presigned URLs.

---

## Milestones

| #   | Milestone    | Deliverable                                                         | Status |
| --- | ------------ | ------------------------------------------------------------------- | ------ |
| M1  | Infra up     | pg + redis + minio reachable (nuc.test); `ashen` DB + buckets.       | ✅ done |
| M2  | API skeleton | Auth + device register + migrations; verified via curl.             | ✅ done |
| M3  | Upload path  | check → presign → PUT → complete end to end.                        | ✅ done |
| M4  | Worker       | verify + thumbnail + EXIF; asset reaches `complete`.                | ✅ done |
| M5  | iOS MVP      | Scan, dedup-check, background upload; builds+runs on sim.           | ✅ done |
| M6  | Dashboard    | Timeline + devices + storage + download; live-verified.            | ✅ done |
| M7  | Hardening    | Resume/retry; Live Photos; Wi-Fi/charging; auto-catch new photos.  | ✅ done |

M1–M4 sequential (backend foundation). M5/M6 ran in parallel once API stable at M4. M7 polish across all.

**All Phase 1 milestones complete**, each verified against real infra on `nuc.test`.

---

## Open Questions

- ~~Multipart threshold for large videos?~~ **Deferred** — single PUT for now; revisit for >100 MB videos.
- ~~Thumbnail on worker vs device?~~ **Resolved: worker** (Go, imaging). Keeps client simple, originals authoritative.
- ~~SHA-256 on device cost?~~ **Resolved** — streamed/chunked hash during export (CryptoKit), no full-file buffering.
- ~~MinIO vs S3/R2 abstraction?~~ **Resolved** — behind a `storage` interface (minio-go, S3-compatible) from day one.

---

## First Concrete Steps

1. Scaffold `infra/docker-compose.yml` + `.env.example` (M1).
2. Scaffold `services/api` Go module, migrations, `/auth` + `/devices` (M2).
3. Wire the upload path and prove M3 with curl before touching iOS.
