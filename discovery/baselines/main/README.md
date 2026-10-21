# discovery/baselines/main/

The one place drift detection is documented. `pascha drift` hits the
endpoints in [`discovery/probe/manifest.toml`](../../probe/manifest.toml),
reduces each response to a keys-only shape (no values, IDs or PII) and
diffs it against the `.shape` files here. A `.shape` file is committed
per endpoint, an exception to the default operator-only posture for
`discovery/baselines/` (see [`PROTOCOL.md`](../../PROTOCOL.md)).

## What a shape holds

- Every item of every array and every level of nesting is walked; there
  is no depth or sample cap. Array elements merge into one union shape.
- Leaves are `str`, `int`, `float`, `bool`, `null`; an empty array is
  `<empty>`. `<empty>` and `null` match any shape, so a week whose feed
  happens to carry photos or rich text is not drift.
- `--anonymize` writes array lengths as `<n=*>`, so household
  cardinality never reaches a committed file or the public job log.
- A response filter drops keys pascha's structs do not declare, so the
  diff reports removed keys and type changes, never new vendor fields.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Shapes match. |
| 1 | Drift found. |
| 2 | Login or transport failure, or a `.shape` file is missing or unreadable for any manifest endpoint. |

## CI

`drift-gate` in `.github/workflows/drift.yml` runs
`pascha drift --anonymize --diff discovery/baselines/main` on every tag,
weekly, and on demand from the Actions tab: two GETs, one second apart,
on the operator's own account (ADR 0007). It needs the `FAMLY_EMAIL`
and `FAMLY_PASSWORD` repository secrets; a failure email means Famly
drifted or login broke. The fresh shapes are kept for a week as the
run artifact `drift-shapes`.

## Seeding

Reseed whenever the walk changes what a shape holds (the first seed
after deeper shapes landed, or a new manifest endpoint). Run the
workflow, download the artifact, check every key is a json tag of a pascha
struct and every value is a type name, `<empty>`, `<n=*>`, `<object>` or
`<array>`, and copy the files here. Prefer a week whose
first posts include an image, a tagged image and a video, so those
subtrees carry real shapes rather than `<empty>`.

Locally, with your own Famly credentials:

```bash
export FAMLY_EMAIL=...
export FAMLY_PASSWORD=...
pascha drift --anonymize --out-dir discovery/baselines/main
```

Always seed with `--anonymize`; without it the next run reports
cardinality drift on every array. `FAMLY_ACCESS_TOKEN` works for a
one-off run but expires.

## What stays out of this directory

- HAR captures: `discovery/captures/`, gitignored.
- Full schema dumps: `discovery/baselines/__schema.json`, gitignored.
- Operator-private probe output: `discovery/baselines/<other>/`,
  gitignored.
