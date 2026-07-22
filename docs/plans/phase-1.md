# Phase 1 — Reliable Backup

> Goal: an iPhone app that automatically and reliably backs up photos and videos to self-hosted storage at original quality, with a web dashboard to see what's backed up.

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

- [ ] `docker-compose.yml`: postgres, redis, minio, api, worker.
- [ ] Postgres init + volume.
- [ ] MinIO with buckets `photos`, `videos`, `thumbnails` auto-created on boot.
- [ ] `.env.example` for secrets (DB URL, MinIO keys, JWT secret).
- [ ] Makefile targets: `make up`, `make down`, `make migrate`, `make seed`.

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

- [ ] `POST /auth/register`, `POST /auth/login` → JWT.
- [ ] `POST /devices` — register a device, return device id.
- [ ] `POST /uploads/check` — client sends `{sha256, byte_size}[]`; server replies which already exist (dedup) and which to upload.
- [ ] `POST /uploads` — create an upload, return presigned PUT URL(s) (multipart for large videos).
- [ ] `POST /uploads/:id/complete` — client signals done; enqueue verify job.
- [ ] `GET /assets` — paginated timeline (by `captured_at`).
- [ ] `GET /assets/:id/original` — presigned GET.
- [ ] `GET /assets/:id/thumb` — presigned GET.
- [ ] `GET /stats` — counts + storage usage.

**Cross-cutting:**

- [ ] JWT middleware, request logging, structured errors.
- [ ] SQL migrations (goose or migrate).
- [ ] Presigned URL helper (MinIO SDK, S3-compatible).

### 3. Worker (`services/worker/`, Go)

- [ ] Consume `verify` jobs from Redis.
- [ ] Download uploaded object, recompute SHA-256, compare to claimed hash. Mismatch → mark `failed`, delete object.
- [ ] Generate thumbnail (JPEG, longest edge ~512px). HEIC decode support.
- [ ] Extract EXIF (captured_at, dimensions, GPS) into `assets.exif`.
- [ ] Mark asset `complete`.
- [ ] Live Photo handling: link still + video via `live_photo_group_id`.

### 4. iOS (`apps/ios/`, SwiftUI)

- [ ] Photos permission request + onboarding.
- [ ] `PHPhotoLibrary` scan → enumerate assets, compute SHA-256 of originals.
- [ ] Local queue (Core Data / SQLite): pending, in-flight, done.
- [ ] `POST /uploads/check` to skip already-backed-up assets.
- [ ] Background `URLSession` upload to presigned URLs; resume on relaunch.
- [ ] Settings: Wi-Fi-only, charging-only toggles.
- [ ] `PHPhotoLibraryChangeObserver` to catch new photos automatically.
- [ ] Live Photo upload (still + paired video).
- [ ] Status UI: backed up / remaining / current.

### 5. Web (`apps/web/`, Next.js)

- [ ] Login (JWT stored client-side).
- [ ] Timeline grid (virtualized, grouped by day), thumbnails from `/thumb`.
- [ ] Device list + last-seen + per-device upload progress.
- [ ] Storage usage + asset counts (`/stats`).
- [ ] Download original.

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

| #   | Milestone    | Deliverable                                                         |
| --- | ------------ | ------------------------------------------------------------------- |
| M1  | Infra up     | `docker compose up` runs pg + redis + minio; buckets exist.         |
| M2  | API skeleton | Auth + device register + migrations; hit endpoints via curl.        |
| M3  | Upload path  | check → presign → PUT → complete works end to end for one file.     |
| M4  | Worker       | verify + thumbnail + EXIF; asset reaches `complete`.                |
| M5  | iOS MVP      | Scan, dedup-check, background upload of a real library.             |
| M6  | Dashboard    | Timeline + device status + storage + download.                      |
| M7  | Hardening    | Resume/retry under network loss; Live Photos; Wi-Fi/charging modes. |

M1–M4 are sequential (backend foundation). M5 and M6 run in parallel once the API is stable at M4. M7 is polish across all.

---

## Open Questions

- Multipart threshold for large videos? (Propose: multipart above ~100 MB.)
- Thumbnail generation on worker (Go) vs. on device before upload? (Propose: worker, to keep client simple and originals authoritative.)
- SHA-256 on device for the full original — acceptable battery/time cost for large videos? (Propose: hash during background processing, chunked.)
- MinIO vs. direct S3/R2 abstraction — build behind one storage interface from day one.

---

## First Concrete Steps

1. Scaffold `infra/docker-compose.yml` + `.env.example` (M1).
2. Scaffold `services/api` Go module, migrations, `/auth` + `/devices` (M2).
3. Wire the upload path and prove M3 with curl before touching iOS.
