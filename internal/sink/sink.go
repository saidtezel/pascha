// Package sink defines the destinations pascha writes assets to.
//
// Immich is the only sink: pascha keeps no local archive. Assets
// are staged in a temp file, prepared (EXIF reinjected, sidecar
// built) and uploaded; the temp file is removed afterwards. New
// sinks implement the same interface and slot into the
// orchestration loop without changes to the asset transitions.
//
// See ADR 0006 and ADR 0008.
package sink

import (
	"context"
	"time"
)

// PutInput is the pascha-friendly request shape. Assembled by the
// orchestration loop from the typestate Prepared value.
type PutInput struct {
	// FamlyImageID is the vendor-stable identifier; used for
	// dedup keys, log lines, and metadata.
	FamlyImageID string

	// Source is "feed-image" or "feed-video".
	Source string

	// FeedItemID is the parent post identifier.
	FeedItemID string

	// SourcePath is the path to the prepared bytes in a temp file.
	SourcePath string

	// Filename is the bare filename including extension. Its
	// extension drives the extension of the uploaded name.
	Filename string

	// SHA1 of the file bytes as downloaded (hex).
	SHA1 string

	// FileCreatedAt is the canonical asset timestamp; Immich gets
	// it as fileCreatedAt.
	FileCreatedAt time.Time

	// Sidecar is an optional standalone XMP packet; Immich gets it
	// as sidecarData. Videos only: photos embed their XMP.
	Sidecar []byte
}

// Receipt describes what a sink did with a Put.
type Receipt struct {
	// DestPath is the vendor asset id (Immich).
	DestPath string

	// Status is "created" for new uploads, "duplicate" when the
	// sink already had the asset.
	Status string

	// Size is the byte count of the asset as the sink saw it.
	Size int64
}

// Sink is the destination interface. Implementations must be safe
// for the orchestration loop to call sequentially per-asset; they
// are not required to be safe for concurrent calls on the same
// asset.
type Sink interface {
	// Put stores the asset and returns a sink-specific receipt.
	// Successful Put implies the artefact is durable for that sink.
	Put(ctx context.Context, in PutInput) (Receipt, error)

	// Name returns a stable identifier ("immich") used in logs.
	Name() string
}
