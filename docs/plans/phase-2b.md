# Phase 2b — Metadata Worker

> Goal: a separate, async worker that extracts, normalizes, enriches, and indexes metadata for **already-verified** assets, powering search by filename, date, camera, dimensions, duration, location, visible text (OCR), captions/labels, and semantic vectors. Never touches originals; never affects backup status.

> **Status: 📝 PLANNED** — new independent service `services/metadata-worker`. Builds on the existing backup pipeline (assets reach `status=complete`). Requirement: `docs/requirements/metadata.md`.

---

## Scope & Sequencing

Delivered in the requirement's order — ship Phase 1 first, it's fully doable on our current stack:

- **M1 (req Phase 1) Technical metadata** — EXTRACT + NORMALIZE (image EXIF, video probe), raw JSON, job status, retries, filename/date search. **Near-term target.**
- **M2 (req Phase 2) Location** — GPS normalize → reverse geocode (Nominatim) → city/province/country search + cache.
- **M3 (req Phase 3) OCR** — screenshot/doc text → full-text search. Off by default.
- **M4 (req Phase 4) AI enrichment** — captions, labels, embeddings (pgvector) → semantic search. Off by default.

Each milestone is independently shippable; later ones enrich the search document without blocking earlier search.

---

## Key Design Decisions (mapped to our stack)

1. **Separate service** `services/metadata-worker` (Go) — leaves the verify/thumbnail/replication worker untouched (thumbnails are explicitly out of scope). Reuses our config pattern (`DATABASE_URL`, `REDIS_URL`, MinIO env).
2. **Queue = Asynq** (Redis-backed). The requirement's queue needs (delayed retries, exponential backoff, priorities, concurrency limits, dedup, dead-letter, graceful shutdown, timeouts) are all first-class in Asynq — far less to hand-roll than our raw `LPUSH`/`BRPOP` lists. Keep the existing verify/replicate lists as-is; metadata runs on its own Asynq queues.
3. **Trigger** — when an asset reaches `complete`, enqueue the metadata pipeline. The existing verify worker's `markComplete` (or the API `/complete`) enqueues an `EXTRACT_METADATA` task. **Idempotent** via a unique `asset_processing_jobs (asset_id, job_type, worker_version)` — duplicate backup events don't create duplicate active jobs (BR-006, FR-001).
4. **Tools** — `exiftool` (images incl. HEIC, better than our current goexif), `ffprobe`/`ffmpeg` (video), `libheif`. Baked into the worker's Dockerfile. Our existing Go `goexif` stays only for the thumbnail worker's cheap capture-date guess; the metadata worker is the authoritative source.
5. **Search** — Postgres native: `tsvector` + GIN for full-text (filename/OCR/caption/labels), `pgvector` (`ivfflat`) for semantic. No Elasticsearch/OpenSearch (non-goal).
6. **Privacy-first defaults** — OCR, content analysis, embeddings, and any external AI provider are **disabled by default**. Enabled per-deployment via config. GPS + OCR text treated as sensitive; logs never dump them.
7. **Independence** — metadata status is fully decoupled from backup. A `complete` asset with `metadata_status=FAILED` stays downloadable + Free-Up-Space eligible (BR-002, BR-011).

---

## Data Model (migrations `0007`+)

Per the requirement's suggested model. New tables:

```
asset_processing_jobs   (id, asset_id, job_type, status, priority, attempts, max_attempts,
                         dependency_job_ids, worker_version, input_hash, error_code,
                         error_message, scheduled_at, started_at, completed_at, next_retry_at, …)
asset_metadata          (asset_id, raw_metadata_json, normalized_metadata_json, metadata_source,
                         metadata_worker_version, normalization_schema_version, extracted_at, normalized_at)
asset_technical_metadata(asset_id, original_filename, mime_type, container_format, file_size,
                         width, height, duration_ms, orientation, frame_rate, bitrate,
                         video_codec, audio_codec, camera_make, camera_model, lens_make, lens_model,
                         iso, aperture, exposure_time, focal_length,
                         captured_at, captured_at_source, captured_at_confidence, captured_timezone,
                         latitude, longitude, altitude)
asset_locations         (asset_id, lat, lng, altitude, country, country_code, province, city,
                         district, neighborhood, postal_code, place_name, geocoding_provider,
                         geocoding_version, geocoded_at)
asset_ocr_results       (id, asset_id, extracted_text, detected_language, confidence,
                         bounding_boxes_json, provider, model, model_version, input_hash, processed_at)
asset_analysis          (id, asset_id, short_caption, detailed_caption, classification,
                         provider, model, model_version, input_hash, processed_at)
asset_labels            (id, asset_id, label, normalized_label, confidence, source, provider, model_version)
asset_embeddings        (id, asset_id, embedding vector, provider, model, dimensions, input_hash, generated_at)
asset_search_documents  (asset_id, searchable_text, search_vector tsvector, indexed_metadata_json,
                         index_version, indexed_at)
```

- `CREATE EXTENSION vector;` on the `ashen` DB (M4).
- Unique index `(asset_id, job_type, worker_version)` on jobs (dedup).
- GIN index on `search_documents.search_vector`; `ivfflat` on `embeddings.embedding` (M4).
- `asset_labels.source` ∈ {USER, EXIF, OCR, AI_ANALYSIS, SYSTEM}. **User labels never overwritten by generated** (BR-009).

`assets.captured_at`/`width`/`height` (already populated cheaply by the thumbnail worker) get **superseded** by the normalized technical metadata once available — the timeline reads normalized values, falling back to the existing columns.

---

## Job Orchestration

Task types (Asynq): `EXTRACT_METADATA`, `NORMALIZE_METADATA`, `REVERSE_GEOCODE`, `RUN_OCR`, `ANALYZE_CONTENT`, `GENERATE_EMBEDDING`, `INDEX_SEARCH_DOCUMENT`.

Dependencies (each completed job enqueues its dependents):

```
EXTRACT_METADATA → NORMALIZE_METADATA ─┬─ REVERSE_GEOCODE (if valid coords)
                                        └─ INDEX_SEARCH_DOCUMENT (re-runs as enrichment lands)
RUN_OCR ────────────┐
ANALYZE_CONTENT ────┴→ (feed INDEX) → GENERATE_EMBEDDING (after OCR+analysis settle)
```

- `NORMALIZE` requires successful `EXTRACT`. `REVERSE_GEOCODE` requires valid normalized coords.
- `RUN_OCR` / `ANALYZE_CONTENT` run independently after verification (when enabled).
- `INDEX_SEARCH_DOCUMENT` is idempotent + re-runnable as enrichment completes (partial results, BR-010).
- `GENERATE_EMBEDDING` runs after OCR + analysis complete/fail-permanent/skip.

**Asset `metadata_status`** derived from job rows: NOT_STARTED / PENDING / PROCESSING / PARTIALLY_COMPLETED (core done, enrichment pending) / COMPLETED / FAILED / SKIPPED (FR-015).

**Retries** (Asynq per-type): 1m → 5m → 30m → 2h → 12h; permanent-fail for unsupported/corrupt/missing-object/invalid-coords; OCR-no-text = success with empty result. Dead-letter (archive) + alert on missing original.

**Concurrency per type** (config): EXTRACT 10, GEOCODE 5, OCR 2, ANALYZE 2, EMBED 5 — CPU-heavy jobs on separate Asynq queues so they don't block light extraction.

---

## Workstreams

### M1 — Technical metadata (req Phase 1)

- [ ] Scaffold `services/metadata-worker` (Go, Asynq server, config, pgx pool, MinIO client).
- [ ] Migration `0007`: `asset_processing_jobs`, `asset_metadata`, `asset_technical_metadata`.
- [ ] Enqueue `EXTRACT_METADATA` on asset `complete` (idempotent job row).
- [ ] `EXTRACT_METADATA`: stream original from MinIO to a temp file (no full in-memory load); run `exiftool -json` (images) / `ffprobe -print_format json` (video); store `raw_metadata_json`. Missing fields ≠ failure (BR-004).
- [ ] `NORMALIZE_METADATA`: map raw → `asset_technical_metadata` (ISO timestamps, MIME→media type, orientation, DMS→decimal GPS, camera make/model, units). Capture-time priority order + source + confidence (FR-006).
- [ ] `INDEX_SEARCH_DOCUMENT` v1: filename + date + media type + camera into `search_documents.search_vector`.
- [ ] API: `GET /assets/{id}/metadata`, `GET /assets/{id}/processing-jobs`, `POST /assets/{id}/metadata/reprocess`, `POST /metadata-jobs/retry-failed` (owner/admin gated).
- [ ] Timeline filters extended: camera model, dimensions, video duration.
- [ ] Dockerfile with exiftool + ffprobe + libheif; graceful shutdown.

### M2 — Location (req Phase 2)

- [ ] `REVERSE_GEOCODE` (Nominatim default): coords → country/province/city/district/place. Cache by rounded coords (`cache_precision`). Failure retries independently, never fails extraction (FR-008).
- [ ] `asset_locations` + city/province/country search filters + index doc update.

### M3 — OCR (req Phase 3, off by default)

- [ ] `RUN_OCR` (local tesseract default; pluggable). Screenshot/document heuristic gating. Store text/lang/confidence/boxes; no-text = success. Add to search doc (full-text). Sensitive: owner-only.

### M4 — AI enrichment (req Phase 4, off by default)

- [ ] `ANALYZE_CONTENT`: captions + labels (local/external, configurable). Marked `AI_ANALYSIS` source + model version (BR-008).
- [ ] `GENERATE_EMBEDDING`: pgvector, `input_hash` dedup, model/dims recorded. Semantic search endpoint (`<->` cosine).
- [ ] `CREATE EXTENSION vector` + ivfflat index.

---

## API (new)

| Route                                                            | Does                                                      |
| ---------------------------------------------------------------- | --------------------------------------------------------- |
| `GET /assets/{id}/metadata`                                      | technical + location + ocr + analysis + processing_status |
| `GET /assets/{id}/processing-jobs`                               | per-job status                                            |
| `POST /assets/{id}/metadata/reprocess`                           | body `{job_types:[…]}` — requeue selected                 |
| `POST /metadata-jobs/retry-failed`                               | admin/owner — retry all failed                            |
| `GET /search?q=&from=&to=&camera=&city=&country=&min_duration=…` | full-text + faceted (semantic added M4)                   |

Ownership enforced on all; sensitive fields (GPS, OCR text) owner-only (BR-012).

---

## Config (per requirement)

```yaml
metadata_worker:
  enabled: true
  extraction: { enabled: true, tool: exiftool }
  video: { enabled: true, tool: ffprobe }
  reverse_geocoding: { enabled: true, provider: nominatim, cache_precision: 4 }
  ocr: { enabled: false, provider: local }
  content_analysis: { enabled: false, provider: local }
  embeddings: { enabled: false, provider: local, dimensions: 768 }
```

External AI **off by default**; users informed before media leaves the box.

---

## Business Rules → enforcement

| Rule                                           | How                                                           |
| ---------------------------------------------- | ------------------------------------------------------------- |
| BR-001 metadata only after verified            | trigger on `status=complete` only                             |
| BR-002 metadata never affects backup           | separate tables/status; verify worker untouched               |
| BR-003 immutable original                      | worker only GETs the object; writes to temp, deletes after    |
| BR-004 missing optional ≠ failure              | extraction completes with available fields                    |
| BR-005 required vs optional tracked separately | job_type + status per row                                     |
| BR-006 no duplicate active jobs                | unique `(asset_id, job_type, worker_version)`                 |
| BR-007 safe retries                            | upserts by `input_hash`; no dup labels/embeddings/search rows |
| BR-008 AI content marked                       | `source=AI_ANALYSIS` + model version                          |
| BR-009 user metadata wins                      | generated never overwrites `source=USER`                      |
| BR-010 partial search                          | index doc built from whatever completed                       |
| BR-011 failure ≠ blocks download/delete        | download + Free-Up-Space read backup status only              |
| BR-012 sensitive access control                | GPS/OCR owner-only; logs redact                               |

---

## Observability

Metrics: jobs created/completed/failed/retried/processing, avg time per type, queue wait, bytes processed, OCR success rate, geocode cache-hit, AI request count/cost, embeddings count, permanent failures. Structured logs (job id, asset id, type, worker version, attempt, duration, error) — **never** full OCR text / precise GPS / signed URLs.

---

## Performance / Security

- Images <50 MB: technical extract <10s; videos <5 GB: probe <30s. Stream/temp-download, never load whole media in memory.
- Storage access via internal creds or short-lived signed URLs; temp files deleted (encrypted disk where available); metadata access enforces ownership.

---

## Open Questions

- **Asynq adoption** — introduce for metadata only, or migrate verify/replicate too? (Propose: metadata only first; migrate later if it proves out.)
- **HEIC EXIF** — exiftool covers it; keep libheif only if we later need pixel access (OCR/analysis). (Propose: exiftool for M1; libheif when OCR lands.)
- **OCR engine on Linux** — Apple Vision is client-only; server uses tesseract or an external API. (Propose: tesseract default, external pluggable.)
- **Embedding model** — local (privacy) vs external. Dimensions? (Propose: local default, 768-dim, pgvector.)
- **Reprocess authz** — strict owner vs admin-only for bulk. (Propose: per-asset = owner; `retry-failed` bulk = admin.)
- **Timeline source of truth** — switch reads to normalized technical metadata, fall back to existing `assets.captured_at/width/height`.

---

## Deployment

`services/metadata-worker/` independent service + Dockerfile (exiftool, ffmpeg/ffprobe, libheif, optional OCR/AI runtimes). Graceful shutdown drains/releases in-flight Asynq jobs. Runs against the same nuc infra (Postgres `ashen`, Redis, MinIO).

---

## First Concrete Steps (M1)

1. Scaffold `services/metadata-worker` + Asynq server + config + migration `0007` (jobs + metadata + technical tables).
2. Enqueue `EXTRACT_METADATA` on `complete`; implement extract (exiftool/ffprobe) → raw JSON.
3. `NORMALIZE_METADATA` → `asset_technical_metadata`; `INDEX_SEARCH_DOCUMENT` v1; metadata status + `GET /assets/{id}/metadata`.
4. Verify on nuc: back up a real HEIC + a video → metadata rows populated, filename/date/camera search works, backup status unaffected.
