# Feature Requirement: Metadata Worker

## Overview

The **Metadata Worker** extracts, normalizes, enriches, and indexes metadata from photos and videos that have already been successfully backed up to Ashen Photos.

The worker enables users to search and filter backed-up media using information such as:

- filename
- capture date
- media type
- dimensions
- duration
- camera model
- location
- visible text
- generated labels
- natural-language descriptions

Metadata processing must run asynchronously and must not block the primary backup flow.

The existing thumbnail worker is outside the scope of this feature.

---

## Objectives

- Extract technical metadata from backed-up images and videos.
- Preserve original EXIF and media metadata.
- Normalize metadata into searchable database fields.
- Enrich GPS coordinates with human-readable locations.
- Extract visible text from supported media.
- Generate searchable captions and labels.
- Prepare media for semantic search.
- Support retries, reprocessing, and worker versioning.
- Keep metadata processing independent from backup verification.

---

## Non-Goals

The first version does not include:

- thumbnail generation
- image resizing
- video preview generation
- face recognition
- person identification
- photo editing
- destructive modification of original files
- automatic deletion from the user's device
- real-time metadata extraction during upload
- dedicated external search engines such as Elasticsearch or OpenSearch

---

## User Stories

### Search by date

> As a user, I want to search for photos by capture date so I can find media from a specific period.

### Search by location

> As a user, I want to search for photos taken in a city or country so I can find memories from a specific place.

### Search visible text

> As a user, I want to search text visible in screenshots, receipts, and documents so I can find information inside my media.

### Search by description

> As a user, I want to describe what I remember in natural language so I can find relevant photos and videos.

### Filter by technical metadata

> As a user, I want to filter media by camera, file type, dimensions, or duration so I can organize my library.

---

# Architecture

```text
Verified Backup
      │
      ▼
asset.backup.verified event
      │
      ▼
Metadata Job Orchestrator
      │
      ├── Extract Technical Metadata
      ├── Normalize Metadata
      ├── Reverse Geocode
      ├── Run OCR
      ├── Analyze Content
      ├── Generate Embedding
      └── Update Search Index
               │
               ▼
         PostgreSQL
```

The metadata worker must read the original file from object storage without modifying it.

---

# Processing Principles

## Independent Backup and Metadata Status

Backup verification and metadata processing are separate concerns.

A file may be safely backed up even when metadata extraction fails.

```text
Backup status: VERIFIED
Metadata status: FAILED
```

This asset must remain available for download and must remain eligible for the Free Up Space feature, provided the backup itself is verified.

Metadata failure must never change a verified backup back into an unverified state.

---

## Immutable Originals

The metadata worker must never modify, replace, recompress, or delete the original media file.

Extracted metadata must be stored separately in PostgreSQL or another configured metadata store.

---

## Asynchronous Processing

Metadata processing must happen after backup verification.

The upload API must not wait for:

- EXIF extraction
- video probing
- reverse geocoding
- OCR
- AI analysis
- embedding generation
- search indexing

---

# Functional Requirements

## FR-001 Create Metadata Jobs

When an asset reaches the `VERIFIED` backup state, the system shall create the required metadata-processing jobs.

Required initial jobs:

- `EXTRACT_METADATA`
- `NORMALIZE_METADATA`
- `INDEX_SEARCH_DOCUMENT`

Optional enrichment jobs:

- `REVERSE_GEOCODE`
- `RUN_OCR`
- `ANALYZE_CONTENT`
- `GENERATE_EMBEDDING`

Job creation must be idempotent.

Processing the same backup event more than once must not create duplicate active jobs.

---

## FR-002 Extract Image Metadata

For supported image formats, the worker shall extract available technical and EXIF metadata.

Supported initial formats:

- JPEG
- JPG
- PNG
- HEIC
- HEIF
- WebP
- TIFF

The worker should extract, when available:

- original filename
- MIME type
- file size
- width
- height
- orientation
- color space
- bit depth
- capture date
- modification date
- camera manufacturer
- camera model
- lens manufacturer
- lens model
- focal length
- aperture
- ISO
- exposure time
- flash status
- software
- GPS latitude
- GPS longitude
- GPS altitude
- EXIF version

Missing metadata must not cause the job to fail.

---

## FR-003 Extract Video Metadata

For supported video formats, the worker shall extract available technical metadata.

Supported initial formats:

- MOV
- MP4
- M4V
- HEVC
- QuickTime containers supported by the configured media probe

The worker should extract, when available:

- original filename
- MIME type
- container format
- file size
- width
- height
- duration
- rotation
- frame rate
- video codec
- audio codec
- bitrate
- creation date
- modification date
- GPS coordinates
- encoder
- stream information

The worker should use `ffprobe` or an equivalent media inspection tool.

---

## FR-004 Preserve Raw Metadata

The worker shall preserve the original extracted metadata in a raw JSON field.

Example:

```json
{
  "source": "exiftool",
  "version": "13.00",
  "metadata": {
    "Make": "Apple",
    "Model": "iPhone 16 Pro",
    "DateTimeOriginal": "2026:07:20 14:42:31",
    "GPSLatitude": -6.1754,
    "GPSLongitude": 106.8272
  }
}
```

Raw metadata is required for:

- debugging
- future migrations
- re-normalization
- supporting new searchable fields without reading the original file again

Sensitive metadata must only be visible to authorized users.

---

## FR-005 Normalize Metadata

The worker shall convert extracted metadata into consistent application-level fields.

Examples:

| Source value          | Normalized value                 |
| --------------------- | -------------------------------- |
| `2026:07:20 14:42:31` | ISO 8601 timestamp               |
| `image/heic`          | `IMAGE` media type               |
| `QuickTime`           | `VIDEO` media type               |
| EXIF orientation `6`  | 90-degree clockwise orientation  |
| GPS DMS coordinates   | Decimal latitude and longitude   |
| `Apple iPhone 16 Pro` | Normalized camera make and model |

Normalized values must use consistent:

- field names
- units
- timezone handling
- enum values
- date formats
- coordinate formats

---

## FR-006 Determine Capture Time

The system shall determine the most reliable capture timestamp.

Preferred image timestamp order:

1. EXIF original capture time
2. PhotoKit-provided creation time
3. embedded media creation time
4. file creation time
5. upload time

Preferred video timestamp order:

1. QuickTime media creation timestamp
2. PhotoKit-provided creation time
3. container creation timestamp
4. file creation time
5. upload time

The system shall store:

- normalized capture timestamp
- timestamp source
- original timezone, when available
- confidence level

Example:

```json
{
  "captured_at": "2026-07-20T14:42:31+07:00",
  "captured_at_source": "EXIF_DATE_TIME_ORIGINAL",
  "captured_at_confidence": "HIGH"
}
```

---

## FR-007 Extract GPS Coordinates

When location metadata is available, the worker shall store:

- latitude
- longitude
- altitude, when available
- coordinate source
- location accuracy, when available

Coordinates shall be validated before storage.

Invalid coordinates must be ignored and recorded as a processing warning.

---

## FR-008 Reverse Geocode Coordinates

When valid coordinates are available, the system may create a `REVERSE_GEOCODE` job.

The reverse-geocoding result should include:

- country
- country code
- province or state
- city
- district
- neighborhood
- postal code
- human-readable place name
- provider
- provider response version

Example:

```json
{
  "country": "Indonesia",
  "country_code": "ID",
  "province": "DKI Jakarta",
  "city": "Central Jakarta",
  "district": "Gambir",
  "place_name": "Monas"
}
```

The system should cache reverse-geocoding results using rounded coordinates to reduce repeated provider calls.

Reverse-geocoding failure must not fail technical metadata extraction.

---

## FR-009 Run OCR

For supported media, the system may create a `RUN_OCR` job.

Initial OCR targets:

- screenshots
- receipts
- documents
- signs
- whiteboards
- images containing visible text

OCR output should include:

- extracted text
- detected language
- confidence
- OCR provider or model
- OCR version
- bounding boxes, when supported

Example:

```json
{
  "text": "Invoice Number INV-2026-001",
  "language": "en",
  "confidence": 0.94,
  "provider": "vision-framework",
  "version": "1"
}
```

OCR results must be added to the searchable document.

OCR processing must be configurable because it can require significant CPU, GPU, memory, or external API usage.

---

## FR-010 Analyze Media Content

The system may create an `ANALYZE_CONTENT` job to generate:

- short caption
- descriptive caption
- scene labels
- object labels
- document classification
- screenshot classification
- receipt classification
- confidence values
- analysis provider
- model version

Example:

```json
{
  "caption": "A laptop displaying a LangGraph workflow diagram",
  "labels": [
    {
      "name": "laptop",
      "confidence": 0.97
    },
    {
      "name": "workflow diagram",
      "confidence": 0.88
    }
  ]
}
```

AI-generated metadata must be identifiable as generated content rather than original file metadata.

---

## FR-011 Generate Embeddings

The system may create a `GENERATE_EMBEDDING` job after searchable text has been prepared.

The embedding input may include:

- normalized filename
- location
- OCR text
- AI caption
- labels
- camera metadata
- media type
- relevant dates

The system shall store:

- vector embedding
- embedding provider
- model
- dimensions
- input hash
- generation timestamp

The `input_hash` shall prevent regenerating identical embeddings unnecessarily.

PostgreSQL with `pgvector` should be used for the initial implementation.

---

## FR-012 Build Search Document

The system shall create one normalized search document for every asset.

The search document may contain:

- filename
- normalized filename
- media type
- captured date
- camera make
- camera model
- lens model
- city
- province
- country
- OCR text
- generated caption
- generated labels
- user-defined labels
- album names

Example:

```text
IMG_1042.HEIC
Photo captured July 20, 2026
Apple iPhone 16 Pro
Central Jakarta, DKI Jakarta, Indonesia
A laptop displaying a LangGraph workflow diagram
laptop workflow diagram software architecture
```

The search document must be updated when any dependent metadata changes.

---

## FR-013 Support Metadata Reprocessing

Authorized users or system administrators shall be able to request metadata reprocessing for:

- one asset
- multiple selected assets
- all assets using an outdated worker version
- all failed jobs
- all assets missing a specific metadata type

Examples:

```text
Reprocess OCR failures
Rebuild embeddings using model v2
Re-normalize EXIF metadata
Reverse geocode missing locations
```

Reprocessing must not duplicate labels or search records.

---

## FR-014 Support Worker Versioning

Every processing result shall record the version of the worker, tool, provider, or model that created it.

Examples:

- metadata worker version
- ExifTool version
- ffprobe version
- OCR model version
- caption model version
- embedding model version
- normalization schema version

Versioning is required to determine which assets need reprocessing after logic changes.

---

## FR-015 Expose Metadata Status

The API shall expose the processing status for each asset.

Example:

```json
{
  "asset_id": "asset_123",
  "backup_status": "VERIFIED",
  "metadata_status": "PARTIALLY_COMPLETED",
  "jobs": {
    "extract_metadata": "COMPLETED",
    "normalize_metadata": "COMPLETED",
    "reverse_geocode": "COMPLETED",
    "ocr": "FAILED",
    "analyze_content": "PENDING",
    "generate_embedding": "BLOCKED",
    "index_search_document": "COMPLETED"
  }
}
```

The user interface shall distinguish between:

- safely backed up
- metadata processing
- fully searchable
- partially searchable
- metadata processing failed

---

# Job Types

| Job Type                | Purpose                                          |
| ----------------------- | ------------------------------------------------ |
| `EXTRACT_METADATA`      | Read technical and raw metadata                  |
| `NORMALIZE_METADATA`    | Convert extracted values into application fields |
| `REVERSE_GEOCODE`       | Convert coordinates into searchable locations    |
| `RUN_OCR`               | Extract visible text                             |
| `ANALYZE_CONTENT`       | Generate captions and labels                     |
| `GENERATE_EMBEDDING`    | Generate semantic-search vectors                 |
| `INDEX_SEARCH_DOCUMENT` | Build or update searchable asset content         |

Thumbnail-related jobs are handled by the existing thumbnail worker and are not part of this feature.

---

# Job Dependencies

```text
EXTRACT_METADATA
      │
      ▼
NORMALIZE_METADATA
      │
      ├── REVERSE_GEOCODE
      │
      └── INDEX_SEARCH_DOCUMENT
               ▲
               │
RUN_OCR ───────┤
               │
ANALYZE_CONTENT
               │
               ▼
GENERATE_EMBEDDING
```

Recommended dependency behavior:

- `NORMALIZE_METADATA` requires successful metadata extraction.
- `REVERSE_GEOCODE` requires valid normalized coordinates.
- `RUN_OCR` may run independently after backup verification.
- `ANALYZE_CONTENT` may run independently after backup verification.
- `INDEX_SEARCH_DOCUMENT` may run multiple times as enrichment completes.
- `GENERATE_EMBEDDING` should run after OCR and content analysis complete, fail permanently, or are skipped.

---

# Processing States

## Asset Metadata Status

| Status                | Description                                                        |
| --------------------- | ------------------------------------------------------------------ |
| `NOT_STARTED`         | No metadata jobs created                                           |
| `PENDING`             | Jobs created but not started                                       |
| `PROCESSING`          | At least one job is running                                        |
| `PARTIALLY_COMPLETED` | Core metadata completed but optional enrichment remains incomplete |
| `COMPLETED`           | All enabled metadata jobs completed                                |
| `FAILED`              | Required metadata processing failed                                |
| `SKIPPED`             | Metadata processing intentionally disabled or unsupported          |

## Individual Job Status

| Status             | Description                             |
| ------------------ | --------------------------------------- |
| `PENDING`          | Waiting to run                          |
| `BLOCKED`          | Waiting for dependencies                |
| `PROCESSING`       | Currently running                       |
| `COMPLETED`        | Finished successfully                   |
| `FAILED_RETRYABLE` | Failed and eligible for retry           |
| `FAILED_PERMANENT` | Failed and will not retry automatically |
| `SKIPPED`          | Not applicable or disabled              |
| `CANCELLED`        | Manually cancelled                      |

---

# Business Rules

## BR-001

Metadata processing may only begin after the asset backup is verified.

---

## BR-002

Metadata status must not affect backup verification status.

---

## BR-003

The original media object must remain immutable.

---

## BR-004

Missing optional metadata must not be treated as an extraction failure.

For example, an image without GPS coordinates may still complete metadata extraction successfully.

---

## BR-005

Required technical metadata jobs and optional enrichment jobs must be tracked separately.

---

## BR-006

The system must prevent duplicate active jobs for the same asset, job type, and worker version.

---

## BR-007

The system must support safe retries.

Repeated job execution must not create duplicate normalized metadata, labels, embeddings, or search records.

---

## BR-008

AI-generated metadata must be clearly marked with its source and model version.

---

## BR-009

User-provided metadata must never be overwritten by generated metadata.

User-provided titles, descriptions, tags, or location corrections have higher priority than automatically generated values.

---

## BR-010

Search indexing must support partial results.

An asset with completed technical metadata but failed OCR must still be searchable by filename, date, media type, camera, and location.

---

## BR-011

Metadata extraction failures must not prevent users from downloading or deleting the backed-up asset from Ashen Photos.

---

## BR-012

Sensitive metadata, including GPS coordinates and OCR text, must only be accessible to authorized users.

---

# Suggested Data Model

## `asset_metadata`

```sql
asset_metadata
- id
- asset_id
- raw_metadata_json
- normalized_metadata_json
- metadata_source
- metadata_worker_version
- normalization_schema_version
- extracted_at
- normalized_at
- created_at
- updated_at
```

---

## `asset_technical_metadata`

```sql
asset_technical_metadata
- asset_id
- original_filename
- mime_type
- container_format
- file_size
- width
- height
- duration_ms
- orientation
- frame_rate
- bitrate
- video_codec
- audio_codec
- camera_make
- camera_model
- lens_make
- lens_model
- iso
- aperture
- exposure_time
- focal_length
- captured_at
- captured_at_source
- captured_at_confidence
- captured_timezone
- latitude
- longitude
- altitude
- created_at
- updated_at
```

---

## `asset_locations`

```sql
asset_locations
- asset_id
- latitude
- longitude
- altitude
- country
- country_code
- province
- city
- district
- neighborhood
- postal_code
- place_name
- geocoding_provider
- geocoding_version
- geocoded_at
- created_at
- updated_at
```

---

## `asset_ocr_results`

```sql
asset_ocr_results
- id
- asset_id
- extracted_text
- detected_language
- confidence
- bounding_boxes_json
- provider
- model
- model_version
- input_hash
- processed_at
- created_at
- updated_at
```

---

## `asset_analysis`

```sql
asset_analysis
- id
- asset_id
- short_caption
- detailed_caption
- classification
- provider
- model
- model_version
- input_hash
- processed_at
- created_at
- updated_at
```

---

## `asset_labels`

```sql
asset_labels
- id
- asset_id
- label
- normalized_label
- confidence
- source
- provider
- model_version
- created_at
- updated_at
```

Possible `source` values:

- `USER`
- `EXIF`
- `OCR`
- `AI_ANALYSIS`
- `SYSTEM`

---

## `asset_embeddings`

```sql
asset_embeddings
- id
- asset_id
- embedding
- provider
- model
- dimensions
- input_hash
- generated_at
- created_at
- updated_at
```

---

## `asset_search_documents`

```sql
asset_search_documents
- asset_id
- searchable_text
- search_vector
- indexed_metadata_json
- index_version
- indexed_at
- created_at
- updated_at
```

---

## `asset_processing_jobs`

```sql
asset_processing_jobs
- id
- asset_id
- job_type
- status
- priority
- attempts
- max_attempts
- dependency_job_ids
- worker_version
- input_hash
- error_code
- error_message
- scheduled_at
- started_at
- completed_at
- next_retry_at
- created_at
- updated_at
```

---

# Queue Requirements

The initial implementation should use:

```text
Go API
Redis
Asynq
PostgreSQL
Object Storage
```

Kafka is not required for the initial implementation.

The queue system must support:

- delayed retries
- exponential backoff
- job priorities
- concurrency limits
- job deduplication
- dead-letter handling
- graceful shutdown
- job timeouts
- visibility into failed jobs

---

# Retry Strategy

Recommended retry behavior:

| Failure                                | Retry                                  |
| -------------------------------------- | -------------------------------------- |
| Object storage temporarily unavailable | Yes                                    |
| Database connection failure            | Yes                                    |
| Reverse-geocoding rate limit           | Yes                                    |
| External AI provider unavailable       | Yes                                    |
| Unsupported media format               | No                                     |
| Corrupted file                         | No, after validation                   |
| Missing original object                | No, alert immediately                  |
| Invalid coordinates                    | No                                     |
| OCR produced no text                   | No failure; complete with empty result |

Suggested retry delays:

```text
Attempt 1: 1 minute
Attempt 2: 5 minutes
Attempt 3: 30 minutes
Attempt 4: 2 hours
Attempt 5: 12 hours
```

Retry behavior should be configurable per job type.

---

# Error Handling

| Scenario                                   | Expected Behavior                                      |
| ------------------------------------------ | ------------------------------------------------------ |
| Original object is missing                 | Mark job permanently failed and raise an alert         |
| Original object is temporarily unavailable | Retry                                                  |
| Image has no EXIF metadata                 | Complete successfully with available fields            |
| File is corrupted                          | Mark extraction as permanently failed                  |
| GPS value is invalid                       | Ignore coordinates and record warning                  |
| Reverse-geocoding provider fails           | Retry independently                                    |
| OCR finds no text                          | Complete successfully with an empty result             |
| AI provider rate-limits requests           | Retry with backoff                                     |
| Embedding generation fails                 | Preserve text search and retry embedding independently |
| Search indexing fails                      | Retry without rerunning successful extraction jobs     |
| Worker crashes during processing           | Job becomes retryable after timeout                    |
| Same event is received twice               | Do not create duplicate active jobs                    |

---

# API Requirements

## Retrieve Metadata

```http
GET /assets/{assetId}/metadata
```

Example response:

```json
{
  "asset_id": "asset_123",
  "technical": {
    "media_type": "IMAGE",
    "mime_type": "image/heic",
    "width": 4032,
    "height": 3024,
    "camera_make": "Apple",
    "camera_model": "iPhone 16 Pro",
    "captured_at": "2026-07-20T14:42:31+07:00"
  },
  "location": {
    "city": "Central Jakarta",
    "province": "DKI Jakarta",
    "country": "Indonesia"
  },
  "ocr": {
    "status": "COMPLETED",
    "text": "LangGraph workflow"
  },
  "analysis": {
    "status": "COMPLETED",
    "caption": "A laptop displaying a workflow diagram",
    "labels": ["laptop", "workflow diagram"]
  },
  "processing_status": "COMPLETED"
}
```

---

## Retrieve Processing Jobs

```http
GET /assets/{assetId}/processing-jobs
```

---

## Reprocess Metadata

```http
POST /assets/{assetId}/metadata/reprocess
```

Example request:

```json
{
  "job_types": ["RUN_OCR", "GENERATE_EMBEDDING"]
}
```

---

## Reprocess Failed Jobs

```http
POST /metadata-jobs/retry-failed
```

The endpoint must require administrative authorization or strict user ownership checks.

---

# Search Requirements

The metadata worker shall prepare data for the following initial search capabilities:

- filename search
- date filtering
- media type filtering
- image dimension filtering
- video duration filtering
- camera model filtering
- city filtering
- province filtering
- country filtering
- OCR full-text search
- label search
- generated-caption search
- semantic vector search

Example searches:

```text
photos from Bandung
screenshots about LangGraph
receipts from July
videos longer than five minutes
photos taken with iPhone 16 Pro
documents containing invoice 2026
```

---

# Observability Requirements

The system shall expose metrics for:

- jobs created
- jobs completed
- jobs failed
- jobs retried
- jobs currently processing
- average processing time by job type
- queue waiting time
- file size processed
- OCR success rate
- reverse-geocoding cache hit rate
- AI provider request count
- AI provider cost
- embedding generation count
- permanently failed jobs

Logs must include:

- job ID
- asset ID
- job type
- worker version
- attempt number
- processing duration
- error code
- error message

Logs must not expose full OCR text, precise GPS data, access tokens, or signed object-storage URLs by default.

---

# Performance Requirements

## Technical Metadata

- Images under 50 MB should normally complete technical metadata extraction within 10 seconds.
- Videos under 5 GB should normally complete probing within 30 seconds.
- The worker must stream or temporarily download files safely without loading large media entirely into memory.

## Throughput

The worker must support configurable concurrency by job type.

Example:

```text
EXTRACT_METADATA: 10 concurrent jobs
REVERSE_GEOCODE: 5 concurrent jobs
RUN_OCR: 2 concurrent jobs
ANALYZE_CONTENT: 2 concurrent jobs
GENERATE_EMBEDDING: 5 concurrent jobs
```

CPU-heavy jobs must not block lightweight technical metadata jobs.

---

# Security and Privacy Requirements

- All object-storage access must use authenticated internal credentials or short-lived signed URLs.
- Media files must not be exposed publicly.
- Temporary downloaded files must be removed after processing.
- Temporary files should use encrypted disks where available.
- GPS coordinates must be treated as sensitive user data.
- OCR text must be treated as private user content.
- External AI processing must be configurable and disabled by default for privacy-first deployments.
- Users must be informed when personal media is sent to external providers.
- Provider credentials must be stored securely.
- Logs must not contain extracted private content by default.
- Metadata access must enforce asset ownership.

---

# Configuration Requirements

Administrators shall be able to enable or disable:

- technical metadata extraction
- reverse geocoding
- OCR
- AI content analysis
- embeddings
- external processing providers
- local processing models

Example:

```yaml
metadata_worker:
  enabled: true

  extraction:
    enabled: true
    tool: exiftool

  video:
    enabled: true
    tool: ffprobe

  reverse_geocoding:
    enabled: true
    provider: nominatim
    cache_precision: 4

  ocr:
    enabled: false
    provider: local

  content_analysis:
    enabled: false
    provider: local

  embeddings:
    enabled: false
    provider: local
    dimensions: 768
```

---

# Deployment Requirements

The metadata worker should be deployable as an independent service.

```text
services/
  metadata-worker/
    cmd/
    internal/
      extraction/
      normalization/
      geocoding/
      ocr/
      analysis/
      embeddings/
      indexing/
      jobs/
    Dockerfile
```

Required runtime dependencies may include:

- ExifTool
- FFmpeg and ffprobe
- libheif
- OCR runtime
- configured local or external AI clients

The worker must support graceful shutdown and complete or safely release active jobs during deployment.

---

# Acceptance Criteria

- A metadata job is created after an asset backup is verified.
- Duplicate backup events do not create duplicate active jobs.
- Image EXIF metadata is extracted and stored.
- Video technical metadata is extracted and stored.
- Raw metadata is preserved as JSON.
- Metadata is normalized into consistent database fields.
- Capture timestamps use a documented priority order.
- Valid GPS coordinates are stored.
- Reverse-geocoding failures do not fail technical extraction.
- Assets without EXIF metadata still process successfully.
- OCR results become searchable when OCR is enabled.
- Generated captions and labels record their source and model version.
- Embeddings are stored with model and input-hash information.
- Search documents update as enrichment jobs complete.
- Failed jobs can be retried independently.
- Successful jobs are not unnecessarily rerun.
- Metadata processing failure does not change backup verification status.
- The original asset remains unchanged.
- Users can retrieve metadata-processing status through the API.
- Existing thumbnail-worker behavior remains unaffected.

---

# Implementation Order

## Phase 1: Technical Metadata

- `EXTRACT_METADATA`
- `NORMALIZE_METADATA`
- image metadata
- video metadata
- raw metadata storage
- job status
- retry support
- filename and date search

## Phase 2: Location Search

- GPS normalization
- reverse geocoding
- location cache
- city, province, and country search

## Phase 3: OCR

- screenshot and document detection
- OCR extraction
- OCR full-text search
- OCR retry controls

## Phase 4: AI Search Enrichment

- generated captions
- generated labels
- embeddings
- semantic search
- model versioning and reprocessing

---

# Future Enhancements

- video frame analysis
- speech transcription
- landmark recognition
- duplicate detection using perceptual hashes
- user-corrected locations
- user-editable captions
- offline local AI processing
- face clustering with explicit user consent
- metadata export
- metadata sidecar files
- search-index migration to OpenSearch when required
