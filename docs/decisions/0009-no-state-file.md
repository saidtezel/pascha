# ADR 0009: no state file; Immich is the record

**Status**: accepted, 2026-10-21. Supersedes
[ADR 0004](./0004-state-as-json-file.md) and the state-file parts of
[ADR 0008](./0008-immich-only.md).

## Context

After ADR 0008, `state.json` recorded one fact per asset: whether
Immich had confirmed it. Immich already knows that. pascha is meant
to run in Docker containers that may have no persistent storage,
and there a state file is worse than none: it starts empty on every
run, so it skips nothing while costing a lock, a flush cadence and a
volume to configure.

Without any record, a run would re-download and re-upload the whole
walked feed and rely on Immich's checksum dedupe. That dedupe keys
on the bytes pascha uploads, which carry reinjected EXIF; anything
that changes those bytes (a new pascha version in the Software tag,
an edited caption, a different `--tz`) would make Immich store a
second copy.

## Decision

Ask Immich before downloading, by the Famly id in the file name.

- Every upload is named by `--filename-pattern`, which must contain
  `{{.ID}}` (`sink.NewImmich` refuses one without it). The default
  pattern always did.
- For each asset that passes the source filter, `sink.Immich.Has`
  calls `POST /search/metadata` with `originalFileName` set to the
  Famly id and `withDeleted: true`. Immich matches that as a
  case-insensitive substring; pascha then requires the id as a whole
  token in a returned name, so `img-1` never matches `img-10`.
- A hit is skipped with no download. A miss is downloaded, prepared
  and uploaded. There is no `Recorded` phase: `Uploaded` is terminal.
- `internal/state`, `PASCHA_STATE_PATH`, the file lock and
  `pascha status` are removed.
- A failed lookup counts like a failed upload toward the run's
  upload gate; only a successful upload resets the count, since a
  lookup precedes every upload.
- The API key needs `asset.read` as well as `asset.upload`.
  `pascha smoke immich` checks that a fresh upload is found by the
  same search.

## Consequences

- Nothing to mount: a fresh container behaves like a long-lived host.
- Dedupe no longer depends on byte-identical re-uploads. A changed
  caption or pascha version does not duplicate an asset already in
  Immich.
- Trashed assets count as present, so trashing a photo in Immich
  does not bring it back. Once the trash is emptied (or an asset is
  deleted permanently), a run whose feed walk still reaches it
  uploads it again.
- Uploads made under a pattern without the id (impossible from
  pascha, possible from other tools) are not recognised; the
  checksum dedupe remains the backstop for identical bytes.
- One search request per candidate asset per run. A default run
  walks 3 feed pages, so this is small; a full backfill
  (`--max-pages 0`) makes one search per asset in the feed.
- No single-writer lock. Two concurrent runs can race on an asset;
  both upload identical bytes and Immich answers the second with
  `duplicate`.

## Alternatives considered

- **Keep `state.json` on a volume.** Works where storage exists;
  the target deployments may have none.
- **Tag or album membership in Immich.** Needs extra permissions and
  calls per upload, and still needs the Famly id per asset, which
  only the file name or a per-asset metadata read provides.
- **Custom asset metadata (`famlyImageId`).** pascha already writes
  it, but Immich's search API cannot filter on it.
- **List all pascha uploads once per run.** Fewer requests on large
  backfills, but needs a filename prefix every pattern shares and a
  full walk of the library's matches on every small cron run.
