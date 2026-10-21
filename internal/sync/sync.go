// Package sync orchestrates the fetch loop: paginate the vendor
// feed, ask Immich whether each asset is already there, and move the
// new ones through the lifecycle. Immich is the only record of what
// has been done; there is no local state. Pure orchestration; the
// package itself does no I/O of its own beyond the dependencies
// passed in.
package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/saidtezel/pascha/api/famly"
	"github.com/saidtezel/pascha/api/immich"
	"github.com/saidtezel/pascha/internal/asset"
	"github.com/saidtezel/pascha/internal/sink"
)

// Source selects which feed images to download. Mutually exclusive
// by construction; replaces the v0.3.x trio of FeedAll/FeedTagged/
// FeedLiked booleans (where FeedAll's default-true silently masked
// the other two).
type Source string

const (
	// SourceAll downloads every image and video on the feed.
	SourceAll Source = "all"
	// SourceTagged downloads only images tagged with one of the
	// HouseholdChildren. Videos are skipped (Famly does not surface
	// child tags on videos).
	SourceTagged Source = "tagged"
	// SourceLiked downloads only images liked by one of the
	// HouseholdLogins. Videos are skipped.
	SourceLiked Source = "liked"
)

// Validate reports whether s is a known source value.
func (s Source) Validate() error {
	switch s {
	case SourceAll, SourceTagged, SourceLiked:
		return nil
	}
	return fmt.Errorf("sync: unknown source %q (want all|tagged|liked)", string(s))
}

// Options tunes the fetch run.
type Options struct {
	// MaxPages caps the feed walk. 0 means unlimited.
	MaxPages int

	// Zone is the fallback zone for the date of a video in a post
	// with no zone of its own, image or video. nil means time.Local.
	Zone *time.Location

	// DryRun stops short of any actual fetch: assets are
	// enumerated and checked against Immich, but nothing is
	// downloaded or uploaded.
	DryRun bool

	// Source picks the feed filter. Required.
	Source Source

	// HouseholdLogins is the set of login IDs treated as "us" for
	// SourceLiked.
	HouseholdLogins map[string]struct{}

	// HouseholdChildren is the set of child IDs treated as "ours"
	// for SourceTagged.
	HouseholdChildren map[string]struct{}

	// Software is the value for EXIF Software tag, e.g. "pascha 0.1".
	Software string

	// IncludeSystemPosts opts in to processing feed items Famly
	// generates automatically (check-in announcements, sign-out
	// notices, etc.). Off by default; their templated text often
	// isn't what an operator wants embedded as photo captions.
	IncludeSystemPosts bool
}

// Deps are the wired-in collaborators. Immich is required: pascha
// keeps no local archive or state, so an asset is done once Immich
// has it.
type Deps struct {
	Famly  *famly.Client
	Immich *sink.Immich
	Logger *slog.Logger
	HTTP   *http.Client

	gate *uploadGate // set by Run
}

// maxUploadFailures is how many Immich calls (lookups or uploads)
// may fail without a successful upload in between before the run
// stops trying Immich. A lookup that succeeds does not reset the
// count: it precedes every upload, so it would hide a server that
// answers searches but refuses uploads. A rerun picks up
// whatever Immich does not hold, so a dead or refusing server should
// cost seconds, not a download and retry cycle per remaining asset.
const maxUploadFailures = 5

// uploadGate stops Immich lookups and uploads for the rest of a run
// after an authorization failure or maxUploadFailures failures in a
// row.
type uploadGate struct {
	fails   int
	stopped bool
}

func (g *uploadGate) isStopped() bool { return g != nil && g.stopped }

func (g *uploadGate) succeeded() {
	if g != nil {
		g.fails = 0
	}
}

func (g *uploadGate) failed(err error, logger *slog.Logger) {
	if g == nil || g.stopped {
		return
	}
	g.fails++
	if errors.Is(err, immich.ErrUnauthorized) || g.fails >= maxUploadFailures {
		g.stopped = true
		logger.Error("immich uploads stopped for this run; a rerun fetches and uploads the rest",
			"consecutiveFailures", g.fails, "err", err)
	}
}

// Result is the JSON-shaped fetch summary.
type Result struct {
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	PagesWalked int       `json:"pagesWalked"`
	Discovered  int       `json:"discovered"`
	// Skipped counts assets filtered out and assets Immich already
	// holds.
	Skipped  int `json:"skipped"`
	Uploaded int `json:"uploaded"`
	// UploadDuplicates counts Immich uploads the server answered with
	// "duplicate"; they are confirmed, like a fresh upload.
	UploadDuplicates int `json:"uploadDuplicates"`
	// UploadFailed counts assets not confirmed in Immich this run,
	// including those whose lookup failed. A rerun retries them; the
	// CLI exits non-zero while any remain.
	UploadFailed int `json:"uploadFailed"`
	ExifErrors   int `json:"exifErrors"`
	Errors       int `json:"errors"`
	// SystemPostsFiltered counts source-matching images that were
	// skipped because their feed item was system-generated and the
	// run did not pass --include-system-posts. Surfaces the
	// interaction between the system-post default-off rule and the
	// --source filter so operators see what they're missing.
	SystemPostsFiltered int `json:"systemPostsFiltered,omitempty"`
}

// Run performs the fetch loop. Returns the run result and an error
// if the loop terminated abnormally. Per-asset failures are logged
// and counted in Result.Errors but do not abort the loop.
func Run(ctx context.Context, deps Deps, opts Options) (Result, error) {
	if err := opts.Source.Validate(); err != nil {
		return Result{}, err
	}
	if deps.Immich == nil {
		return Result{}, errors.New("sync: Immich sink is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Software == "" {
		opts.Software = "pascha"
	}

	res := Result{StartedAt: time.Now().UTC()}
	deps.gate = &uploadGate{}

	for page, err := range deps.Famly.Pages(ctx) {
		if err != nil {
			res.FinishedAt = time.Now().UTC()
			res.Errors++
			return res, err
		}
		res.PagesWalked++
		for _, item := range page.FeedItems {
			processItem(ctx, deps, opts, item, &res, logger)
		}
		if opts.MaxPages > 0 && res.PagesWalked >= opts.MaxPages {
			logger.Info("max pages reached", "pages", res.PagesWalked, "limit", opts.MaxPages)
			break
		}
	}
	res.FinishedAt = time.Now().UTC()
	logger.Info("fetch complete",
		"pages", res.PagesWalked,
		"source", string(opts.Source),
		"discovered", res.Discovered, "skipped", res.Skipped,
		"uploaded", res.Uploaded, "uploadDuplicates", res.UploadDuplicates,
		"uploadFailed", res.UploadFailed, "exifErrors", res.ExifErrors,
		"systemPostsFiltered", res.SystemPostsFiltered,
		"errors", res.Errors,
		"elapsed", res.FinishedAt.Sub(res.StartedAt))
	// Trap D: a filtered source (tagged|liked) that walked pages
	// but matched nothing is the kind of silent zero-result that
	// hides config drift (e.g. childId rotated, no recent likes).
	// Surface it explicitly above the run summary noise.
	if opts.Source != SourceAll && res.Uploaded+res.UploadDuplicates == 0 && res.Discovered > 0 {
		logger.Warn("fetch matched 0 of N discovered images via source filter",
			"source", string(opts.Source),
			"discovered", res.Discovered,
			"hint", "--max-pages caps the walk; if the matching images are deeper in the feed try --max-pages 0, or widen with --source=all to verify the household context resolves")
	}
	return res, nil
}

// processItem walks one feed item's images and videos, applies the
// source filter, and runs each new asset through the pipeline.
func processItem(ctx context.Context, deps Deps, opts Options, item famly.FeedItem, res *Result, logger *slog.Logger) {
	if item.IsSystemGenerated() && !opts.IncludeSystemPosts {
		// Count any media on system posts toward Skipped so the
		// summary doesn't mislead operators about feed coverage.
		skipped := len(item.Images) + len(item.Videos)
		if skipped > 0 {
			res.Skipped += skipped
			// Trap C: if the operator is filtering by source, count
			// images that the filter would have matched in this
			// system-skipped post. Surfaces in the run summary so
			// operators see when --include-system-posts would
			// expand the result set.
			matched := 0
			if opts.Source != SourceAll {
				for _, img := range item.Images {
					if shouldDownloadImage(img, opts) {
						matched++
					}
				}
			}
			if matched > 0 {
				res.SystemPostsFiltered += matched
			}
			logger.Debug("skipped system-generated post",
				"feedItemId", item.FeedItemID,
				"systemPostTypeClass", item.SystemPostTypeClass,
				"assets", skipped,
				"source_matched", matched)
		}
		return
	}
	for _, img := range item.Images {
		if !shouldDownloadImage(img, opts) {
			res.Skipped++
			continue
		}
		processOne(ctx, deps, opts, asset.DiscoverImage(img, item, opts.Zone), res, logger)
	}
	for _, vid := range item.Videos {
		// A video still transcoding is left for a later run, which
		// picks it up once Famly serves the final file.
		if opts.Source != SourceAll || vid.Transcoding || vid.URL == "" {
			res.Skipped++
			continue
		}
		processOne(ctx, deps, opts, asset.DiscoverVideo(vid, item, opts.Zone), res, logger)
	}
}

// shouldDownloadImage applies the source filter to an image.
func shouldDownloadImage(img famly.Image, opts Options) bool {
	switch opts.Source {
	case SourceAll:
		return true
	case SourceTagged:
		for _, tag := range img.Tags {
			if _, ok := opts.HouseholdChildren[tag.ChildID]; ok {
				return true
			}
		}
		return false
	case SourceLiked:
		for _, like := range img.Likes {
			if _, ok := opts.HouseholdLogins[like.LoginID]; ok {
				return true
			}
		}
		if img.Liked && len(opts.HouseholdLogins) > 0 {
			return true
		}
		return false
	}
	return false
}

// processOne runs the typestate transitions for one asset.
func processOne(ctx context.Context, deps Deps, opts Options, disc asset.Discovered, res *Result, logger *slog.Logger) {
	res.Discovered++
	id := disc.FamlyImageID()

	// Immich stopped answering: don't look up or download what
	// can't be sent.
	if deps.gate.isStopped() {
		res.UploadFailed++
		return
	}

	// Skip if Immich already has it, trashed included. Anything
	// else (new, or a failed earlier attempt) is fetched from Famly.
	immichID, found, err := deps.Immich.Has(ctx, id)
	if err != nil {
		res.Errors++
		res.UploadFailed++
		logger.Error("lookup", "id", id, "err", err)
		deps.gate.failed(err, logger)
		return
	}
	if found {
		res.Skipped++
		logger.Debug("already in immich", "id", id, "immich_id", immichID)
		return
	}

	if opts.DryRun {
		logger.Info("dry-run: would download+upload", "id", id, "source", disc.Source())
		return
	}

	dl, err := disc.Download(ctx, deps.HTTP)
	if err != nil {
		res.Errors++
		logger.Error("download", "id", id, "err", err)
		return
	}
	defer dl.Cleanup()

	prep, err := dl.Prepare(opts.Software)
	if err != nil {
		res.Errors++
		logger.Error("prepare", "id", id, "err", err)
		return
	}
	if prep.ExifError() != "" {
		res.ExifErrors++
		logger.Warn("exif reinjection failed", "id", id, "err", prep.ExifError())
	}

	up, err := prep.Upload(ctx, deps.Immich)
	if err != nil {
		res.Errors++
		res.UploadFailed++
		logger.Error("upload", "id", id, "err", err)
		deps.gate.failed(err, logger)
		return
	}

	deps.gate.succeeded()
	countUpload(res, up.ImmichStatus())
	logger.Info("complete",
		"id", id, "filename", prep.Filename(),
		"immich_id", up.ImmichAssetID(), "immich_status", up.ImmichStatus())
}

// countUpload tallies a confirmed Immich upload.
func countUpload(res *Result, status string) {
	if status == "duplicate" {
		res.UploadDuplicates++
	} else {
		res.Uploaded++
	}
}
