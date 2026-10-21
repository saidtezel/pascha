# ADR 0008: Immich is the only sink

**Status**: accepted, 2026-10-21. Supersedes the disk sink in
[ADR 0006](./0006-sink-abstraction.md). The state-file points below
(and the `Recorded` phase) are superseded by
[ADR 0009](./0009-no-state-file.md).

## Context

pascha (forked from bairn) wrote every asset to a local directory,
always on, and optionally uploaded the saved file to Immich. For a
household that already runs Immich, the local tree is a second copy
to size, back up and prune, and the code paid for it: a disk sink,
save-dir and directory-pattern configuration, an upload-from-disk
recovery path, and a state machine that tracked "saved" and
"uploaded" separately.

## Decision

Immich is the destination, and the only one.

- `sink.Disk` is gone, with `--save-dir`, `PASCHA_SAVE_DIR`,
  `--dir-pattern` and `--no-immich`. `IMMICH_BASE_URL` and
  `IMMICH_API_KEY` are required for `pascha fetch`.
- The asset lifecycle is Discovered → Downloaded → Prepared →
  Uploaded → Recorded. `Prepare` reinjects EXIF into the downloaded
  temp file and builds a video's XMP sidecar; `Upload` sends both;
  `Record` is only reachable from `Uploaded`. The temp file is
  removed after each asset.
- `--filename-pattern` stays: it names the file Immich stores as the
  asset's original file name. Default unchanged, so uploads are
  named as before.
- The state file records Immich confirmation only. An asset without
  `uploadedAt` (new, failed, or saved-but-not-uploaded by an older
  version) is fetched from Famly again on the next run. A stopped
  upload gate (401/403, or five failures in a row) also stops
  downloads for the rest of the run.

## Consequences

- A failed upload costs a re-download from Famly on the next run
  rather than a re-read from disk. Famly's signed URLs are re-minted
  on every feed walk, so the asset stays reachable as long as it is
  on the feed.
- pascha no longer keeps its own copy. The archive's durability is
  Immich's (and the Immich server's backups). An asset Famly removes
  before a successful upload is lost.
- Immich being down means a run uploads nothing and exits 1; there
  is no partial local progress to fall back on.
- The `Sink` interface is kept: a future destination still slots in
  without touching the transitions.

## Alternatives considered

- **Keep disk as an opt-in.** Smallest change, but keeps the
  recovery path, the extra configuration and the two-track state
  that motivated this ADR.
- **Stage to disk, delete after upload.** Equivalent to the temp
  file, with a configurable directory nobody needs.
