# Feature Requirement: Free Up Space

## Overview

The **Free Up Space** feature allows users to safely reclaim storage on their iPhone by deleting photos and videos that have already been successfully backed up to Ashen Photos.

The primary objective is to provide functionality similar to Google Photos' **Free up space**, while ensuring that no media is deleted before its backup has been fully verified.

---

# Objectives

- Reduce iPhone storage usage.
- Never lose user data.
- Preserve original quality backups.
- Build user trust through transparent verification.
- Provide clear visibility into recoverable storage.

---

# Scope

## Included

- Photos
- Videos
- Live Photos
- HEIC
- JPEG
- MOV
- MP4

## Excluded (Phase 1)

- Shared iCloud Albums
- Hidden Album
- Recently Deleted Album
- iCloud Shared Library management
- Automatic deletion without user confirmation

---

# User Story

> As an iPhone user,
>
> I want to safely remove photos and videos that have already been backed up,
>
> so I can free storage without losing my memories.

---

# Functional Requirements

## FR-001 Detect Backed Up Assets

The system shall determine whether each asset has been successfully backed up.

### Success Criteria

An asset is considered **Backed Up** only if:

- Original file upload completed successfully.
- File checksum matches the server checksum.
- Object exists in storage.
- Metadata has been committed to the database.
- Backup verification completed successfully.

---

## FR-002 Backup Status

Each asset shall maintain one of the following states.

| State               | Description                       |
| ------------------- | --------------------------------- |
| NOT_BACKED_UP       | Asset has never been uploaded     |
| UPLOADING           | Upload currently in progress      |
| UPLOADED            | Upload completed but not verified |
| VERIFIED            | Backup integrity confirmed        |
| SAFE_TO_DELETE      | Eligible for deletion             |
| DELETED_FROM_DEVICE | Removed from iPhone               |
| ERROR               | Backup failed                     |

---

## FR-003 Calculate Recoverable Storage

The application shall calculate:

- number of photos
- number of videos
- total storage that can be recovered

Example:

```text
Photos
4,285

Videos
391

Recoverable Space
183.4 GB
```

---

## FR-004 Safe Delete List

The application shall display all assets eligible for deletion.

Users may:

- review assets
- preview assets
- exclude selected assets
- select all

---

## FR-005 Delete Confirmation

Before deleting media, the application shall display:

- number of files
- recoverable storage
- warning message

Example:

> These items have been safely backed up to Ashen Photos.
>
> They will be removed from this iPhone but remain available in your backup.

Deletion must use the iOS system confirmation dialog.

---

## FR-006 Delete Assets

After confirmation:

- delete selected assets using PhotoKit
- update local metadata
- synchronize deletion status with the server

---

## FR-007 Refresh Storage Statistics

After deletion:

- refresh storage usage
- refresh recoverable space
- update dashboard statistics

---

# Business Rules

## BR-001

Assets may only become **SAFE_TO_DELETE** after successful verification.

---

## BR-002

Uploading alone is not sufficient.

Verification must confirm:

- checksum
- storage existence
- metadata persistence

---

## BR-003

Deletion must never occur automatically.

A user must explicitly initiate deletion.

---

## BR-004

Deletion must use Apple's Photos framework.

The application must not bypass system confirmation.

---

## BR-005

If verification fails, the asset shall remain unavailable for deletion.

---

## BR-006

Deleting an asset from the iPhone shall not remove it from Ashen Photos.

---

## BR-007

The server shall retain the original file until the user explicitly deletes it from Ashen Photos.

---

# Error Handling

| Scenario              | Expected Behavior              |
| --------------------- | ------------------------------ |
| Upload failed         | Asset remains NOT_BACKED_UP    |
| Checksum mismatch     | Asset marked ERROR             |
| Storage unavailable   | Asset not deletable            |
| Metadata missing      | Asset not deletable            |
| User cancels deletion | No changes                     |
| Photo already removed | Synchronize state on next scan |

---

# UI Requirements

## Storage Dashboard

Display:

- Total device storage
- Photos
- Videos
- Recoverable storage
- Last backup time

Example:

```text
Storage

Photos
82 GB

Videos
191 GB

Already Backed Up
256 GB

Recoverable Space
241 GB

[ Free Up Space ]
```

---

## Review Screen

Each asset shall display:

- thumbnail
- date
- file size
- media type
- backup status

Actions:

- Select
- Deselect
- Preview

---

## Confirmation Dialog

Display:

- number of selected items
- recoverable space
- warning message

Buttons:

- Cancel
- Delete

---

# Non-Functional Requirements

## Reliability

- No data loss.
- Verification required before deletion.
- Failed operations must be recoverable.

---

## Performance

- Calculate recoverable storage in under 2 seconds for libraries up to 100,000 assets.
- Support background synchronization.

---

## Security

- HTTPS communication only.
- Signed upload requests.
- SHA-256 verification.
- Authenticated API requests.

---

# Future Enhancements

## Smart Cleanup

Recommend assets based on:

- already backed up
- age
- duplicate content
- large videos
- blurry photos

---

## Automatic Cleanup

Optional user-configurable policies:

- Delete after 30 days
- Delete after 90 days
- Delete when device storage is below a threshold
- Delete only on Wi-Fi and charging

Automatic cleanup must remain disabled by default.

---

## AI Recommendations

Examples:

- "You can safely recover 125 GB."
- "These screenshots are over one year old."
- "Large videos are consuming 42 GB."
- "Duplicate photos detected."

---

# Acceptance Criteria

- Original media is backed up before deletion.
- Checksum verification passes.
- Assets are stored successfully on the server.
- Recoverable storage is calculated accurately.
- Users can review assets before deletion.
- Deletion requires explicit user confirmation.
- Deleted assets remain accessible through Ashen Photos.
- Failed backups are never eligible for deletion.
- Dashboard statistics update after cleanup.
