# Phase 2a — Free Up Space

> Goal: let users safely delete iPhone photos/videos that are **verified-backed-up** to Ashen, reclaiming device storage without risking data loss. Mirrors Google Photos "Free up space".

> **Status: ✅ IMPLEMENTED (iOS)** — built on the existing backup pipeline, no server change (reuses `/uploads/check`). Compiles + links; Storage tab + dashboard render. Runtime paths that require iOS system dialogs (Photos permission, delete confirmation) need a real device to fully exercise. Requirement: `docs/requirements/free-up-space.md`.

**Delivered:**

- `BackupItem` += `byteSize`, `shas`, `verified`, `deletedFromDevice`, `safeToDelete`.
- Backup populates size/shas; dedup-skipped items are `verified` immediately; `lastBackupAt` tracked.
- `reconcileFreeSpace()` — marks uploaded items `verified` via batched `/uploads/check`; drops items no longer on device.
- `deleteFromDevice()` — `PHAssetChangeRequest.deleteAssets` (iOS system confirmation, BR-004).
- Storage tab: recoverable space + photo/video counts + last backup; Review sheet (local PhotoKit thumbnails, select/deselect/all, confirm summary → delete).
- Business rules honored: only server-`complete` eligible (BR-001/002/005); device delete never touches server (BR-006/007).
- **Not done:** optional server `/assets/verify-status` + device-deletion sync (2a-6) — deferred; core works without them.

---

## Success Criteria

Done when:

- The app shows a Storage dashboard with recoverable space, photo/video counts, last backup time.
- Only assets the **server has verified** (`status=complete`) are ever eligible for deletion (BR-001, BR-002, BR-005).
- Recoverable-space calc returns in <2s for large libraries (FR-NFR performance).
- User reviews + selects/excludes assets, confirms via the **iOS system dialog**, and assets delete via PhotoKit (BR-003, BR-004).
- Deleting from device never removes the asset from Ashen (BR-006, BR-007).
- Dashboard stats refresh after cleanup (FR-007).

---

## Key Design Decision: what "verified backed up" means

The requirement's `SAFE_TO_DELETE` = server has the asset **fully verified**: checksum matched, object in storage, metadata committed, worker verification done (BR-002). In our system that is exactly `assets.status = 'complete'`.

We already have the authoritative check: **`POST /uploads/check`** returns `exists=true` for a sha256 **only if a `status='complete'`, non-deleted asset exists** (we changed `ExistingHashes` to filter `status='complete' AND deleted_at IS NULL`). So:

> `exists=true` from `/uploads/check` ⟺ VERIFIED ⟺ SAFE_TO_DELETE.

No new verification logic needed — the client just reconciles its local assets against this. (A dedicated read-only endpoint is proposed below only for clarity/perf.)

### State mapping (requirement → our model)

| Requirement state   | Our source of truth                                                      |
| ------------------- | ------------------------------------------------------------------------ |
| NOT_BACKED_UP       | local `BackupItem` absent / `.pending`, server has no complete asset     |
| UPLOADING           | `BackupItem.state = .uploading`                                          |
| UPLOADED            | PUT+`/complete` done, worker not finished (server `status=uploaded`)     |
| VERIFIED            | server `status=complete` (confirmed via check)                           |
| SAFE_TO_DELETE      | VERIFIED **and** asset still present on device                           |
| DELETED_FROM_DEVICE | PhotoKit delete succeeded; local `BackupItem.state = .deletedFromDevice` |
| ERROR               | `BackupItem.state = .failed` (checksum mismatch, etc.)                   |

---

## Why not re-hash on demand

Hashing 100k originals to check backup status would blow the <2s budget. Instead we **persist per-asset backup facts locally** during backup and reconcile verification in the background:

- `BackupItem` already keys by `PHAsset.localIdentifier`. Extend it with `byteSize` and a `verified` flag.
- Recoverable-space calc = sum `byteSize` of locally-known SAFE_TO_DELETE items **from the cached queue** (pure in-memory, well under 2s).
- Verification reconciliation (batch `/uploads/check` over candidate sha256s) runs in the background / on pull-to-refresh, not on the hot path.

---

## Workstreams

### 1. iOS — backup state model

- [ ] Extend `BackupItem`: add `byteSize: Int64`, `verified: Bool` (server-confirmed complete), `deletedFromDevice: Bool`. Add `.verified` / `.deletedFromDevice` to `BackupState` (or derive).
- [ ] Populate `byteSize` during export (already computed in `PhotoScanner.export`).
- [ ] Live Photos: an item is verified only when **all** its parts (still + paired video) are complete.

### 2. iOS — verification reconciliation

- [ ] `FreeSpaceStore` (ObservableObject): reads the persisted backup queue.
- [ ] Reconcile: batch the sha256 of `.done`/`.uploading` items through `POST /uploads/check` (in `Config.checkBatchSize` chunks). `exists=true` → mark `verified = true`.
- [ ] Only assets **still present in the photo library** (verify via `PHAsset.fetchAssets(withLocalIdentifiers:)`) are SAFE_TO_DELETE.
- [ ] Persist reconciliation results so the dashboard is instant next launch.

### 3. iOS — Storage dashboard (UI)

- [ ] New "Storage" screen (Settings tab or its own tab).
- [ ] Show: recoverable space (bytes), # photos, # videos eligible, last backup time.
- [ ] Compute recoverable = Σ `byteSize` of SAFE_TO_DELETE. Instant (cached).
- [ ] `[ Free Up Space ]` button → Review screen.

### 4. iOS — Review + delete (UI + PhotoKit)

- [ ] Review list/grid: thumbnail, date, size, media type, backup status (FR-004, Review Screen).
- [ ] Select-all / deselect / exclude individual assets; preview (reuse `AssetPreviewView`).
- [ ] Confirmation summary: # selected, recoverable space, warning copy (FR-005).
- [ ] Delete via `PHPhotoLibrary.shared().performChanges { PHAssetChangeRequest.deleteAssets(...) }` — iOS shows its **own** system confirmation (BR-004). Never bypass it.
- [ ] On success: mark items `deletedFromDevice`, persist; on cancel: no changes (error table).

### 5. iOS — post-delete refresh + sync

- [ ] Refresh dashboard stats + recoverable space after deletion (FR-007).
- [ ] `PHPhotoLibraryChangeObserver` already registered — reconcile on external library changes (photo already removed → mark deleted).
- [ ] (Optional) Notify server which device deleted the asset (see server §6) — telemetry only; server keeps the file (BR-007).

### 6. Server — optional additions

The core flow needs **no** server change (reuses `/uploads/check`). Optional, for clarity/robustness:

- [ ] `POST /assets/verify-status` — body `{sha256[]}` → `{sha256, verified}[]`. A read-only, intent-revealing alias of the dedup check (doesn't create rows). Cleaner than overloading `/uploads/check` for a read.
- [ ] `POST /assets/{id}/device-deleted` (or extend `asset_devices` with `deleted_at`) — record that a device removed its local copy. Server **retains** the object + row (BR-006, BR-007). Enables "on device N / off device N" UI later.

> Recommend shipping the core with `/uploads/check` first; add `/assets/verify-status` only if the semantic overload bothers us.

---

## Business Rules → implementation

| Rule                                          | How enforced                                           |
| --------------------------------------------- | ------------------------------------------------------ |
| BR-001 SAFE_TO_DELETE only after verification | eligibility gated on server `status=complete`          |
| BR-002 upload alone insufficient              | we check `complete`, not `uploaded`                    |
| BR-003 never auto-delete                      | delete only from explicit user action; no timers       |
| BR-004 use Apple Photos + system dialog       | `PHAssetChangeRequest.deleteAssets` (iOS shows dialog) |
| BR-005 failed verification not deletable      | `.failed`/`uploaded` excluded from SAFE_TO_DELETE      |
| BR-006 device delete ≠ Ashen delete           | client never calls a server-delete; object untouched   |
| BR-007 server retains original                | no server-side deletion in this feature                |

---

## Error Handling (from requirement)

| Scenario                               | Handling                                                                            |
| -------------------------------------- | ----------------------------------------------------------------------------------- |
| Upload failed                          | item `.failed`/absent → NOT_BACKED_UP, not eligible                                 |
| Checksum mismatch                      | worker already marks asset `failed`; client sees not-verified → ERROR, not eligible |
| Storage unavailable / metadata missing | not `complete` → not eligible                                                       |
| User cancels deletion                  | PhotoKit completion = cancelled → no local changes                                  |
| Photo already removed                  | change-observer reconcile marks `deletedFromDevice`                                 |

---

## Performance

- Recoverable-space + counts computed from the **cached local queue** (no network, no hashing) → well under 2s at 100k.
- Verification reconciliation batched in background (`/uploads/check`), not on the stat path.
- Thumbnails in the review grid use presigned `thumb_url` (already cheap) or local PhotoKit thumbnails.

---

## Milestones

| #    | Milestone         | Deliverable                                                        |
| ---- | ----------------- | ------------------------------------------------------------------ |
| 2a-1 | State model       | `BackupItem` gains size/verified/deleted; populated during backup. |
| 2a-2 | Reconciliation    | Background verify via `/uploads/check`; SAFE_TO_DELETE computed.   |
| 2a-3 | Storage dashboard | Recoverable space + counts + last backup, instant.                 |
| 2a-4 | Review + delete   | Review/select/preview → system-dialog delete via PhotoKit.         |
| 2a-5 | Refresh + sync    | Post-delete stats refresh; change-observer reconcile.              |
| 2a-6 | Server (optional) | `/assets/verify-status`, device-deletion sync.                     |

2a-1→2a-2 unblock everything. 2a-3/2a-4 build the UI. 2a-6 is optional polish.

---

## Open Questions

- Recoverable size source: our stored `byteSize` (from backup) vs. `PHAssetResource` file size? (Propose: stored `byteSize` — authoritative, matches what we verified; fall back to PhotoKit for assets we didn't back up.)
- Live Photo eligibility: require both parts verified? (Propose: yes — delete only when still **and** paired video are `complete`.)
- Should we show assets backed up by **another device** but not this one as deletable here? (Propose: no for v1 — only delete what this device can confirm present + verified.)
- "Last backup time" source — track locally (timestamp of last successful `/complete`) or add a server field? (Propose: local timestamp, cheap.)
- Batch size + rate for reconciliation `/uploads/check` on 100k libraries? (Propose: 200/req, background, cache results.)

---

## Out of Scope (per requirement)

Shared iCloud Albums, Hidden/Recently-Deleted albums, iCloud Shared Library, automatic deletion. Future: Smart Cleanup, Automatic Cleanup policies, AI recommendations.

---

## First Concrete Steps

1. Extend `BackupItem` (+`byteSize`, `verified`, `deletedFromDevice`) and populate size during export (2a-1).
2. `FreeSpaceStore` + background reconciliation via `/uploads/check` (2a-2).
3. Storage dashboard reading cached state (2a-3), then Review + PhotoKit delete (2a-4).
