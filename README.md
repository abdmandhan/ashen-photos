# 📸 Ashen Photos

> A self-hosted, privacy-first photo and video backup platform for iPhone.

Ashen Photos automatically backs up photos and videos from iPhone to your own storage instead of relying on iCloud, Google Photos, or other cloud providers.

The goal is to build an open, extensible platform that gives users complete ownership of their memories while providing a modern experience similar to Google Photos.

---

# Vision

Own your memories.

Ashen Photos is designed as a self-hosted backup platform that automatically synchronizes media from iPhone to your personal server or cloud storage.

Unlike traditional cloud photo services, all files belong to you and can be stored wherever you choose.

Future versions will integrate tightly with the Ashen ecosystem, becoming the media layer of your personal second brain.

---

# Goals

- Automatic photo backup
- Automatic video backup
- Preserve original quality
- Preserve metadata (EXIF)
- Support Live Photos
- Resume interrupted uploads
- Deduplicate uploads
- Multiple devices
- Privacy-first
- Self-hosted
- AI-ready architecture

---

# Non Goals (Phase 1)

- Android support
- Photo editing
- Social sharing
- Public albums
- AI search
- Face recognition
- OCR
- Video transcoding

---

# Architecture

```text
                 ┌────────────────────┐
                 │    iPhone App      │
                 │ SwiftUI + PhotoKit │
                 └─────────┬──────────┘
                           │
                Background Upload
                           │
                           ▼
                ┌──────────────────┐
                │      API         │
                │        Go        │
                └─────────┬────────┘
                          │
        ┌─────────────────┼──────────────────┐
        │                 │                  │
        ▼                 ▼                  ▼
 PostgreSQL           Object Storage      Redis
 Metadata          (MinIO / S3 / R2)      Queue
                          │
                          ▼
                  Background Worker
              Thumbnail / Processing
                          │
                          ▼
                  Next.js Web Dashboard
```

---

# Repository Structure

```text
apps/
    ios/
    web/

services/
    api/
    worker/

packages/
    shared/

infra/
    postgres/
    redis/
    minio/
    docker/
```

---

# Technology Stack

## iOS

- Swift
- SwiftUI
- PhotoKit
- Background URLSession
- Core Data / SQLite

## Backend

- Go
- PostgreSQL
- Redis
- MinIO
- Docker

## Frontend

- Next.js
- React
- Tailwind CSS

---

# Core Features

## Phase 1

### Backup

- Automatic upload
- Background upload
- Resume upload
- Retry upload
- Wi-Fi only mode
- Charging only mode

### Media

- Photos
- Videos
- HEIC
- JPEG
- MOV
- MP4
- Live Photos

### Dashboard

- Timeline
- Device list
- Upload status
- Storage usage
- Download original files

---

# Backup Flow

```text
User grants Photos permission
        │
        ▼
Scan photo library
        │
        ▼
Find new assets
        │
        ▼
Queue uploads
        │
        ▼
Background upload
        │
        ▼
Verify checksum
        │
        ▼
Store original file
        │
        ▼
Generate thumbnail
        │
        ▼
Mark backup complete
```

---

# Storage

Media files are stored separately from metadata.

```text
Object Storage
    photos/
    videos/
    thumbnails/

PostgreSQL
    users
    devices
    assets
    uploads
```

---

# Security

- HTTPS only
- Signed upload URLs
- SHA-256 checksum verification
- JWT authentication
- Object storage isolation

Future versions may support:

- End-to-end encryption
- Passkeys
- Multi-user support

---

# Roadmap

## Phase 1 — Reliable Backup

- iPhone application
- Automatic backup
- Background uploads
- Original quality
- Dashboard
- Docker deployment

---

## Phase 2 — Better Photo Experience

- Albums
- Favorites
- Search
- Multiple devices
- Duplicate detection
- External storage replication

---

## Phase 3 — AI

- OCR
- Image embeddings
- Semantic search
- Natural language search
- Smart collections
- Memory timeline

Example:

> "Show my photos from Bali last year."

> "Find receipts from March."

> "Show screenshots related to LangGraph."

---

# Future Integration with Ashen

Ashen Photos is intended to become one module inside the larger Ashen ecosystem.

```text
Ashen

├── Inbox
├── Documents
├── Notes
├── Tasks
├── Projects
├── Photos
├── Videos
└── AI Memory
```

Instead of acting only as a backup application, Ashen Photos will become the permanent storage layer for media inside the user's personal knowledge system.

---

# License

MIT
