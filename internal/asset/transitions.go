package asset

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/saidtezel/pascha/internal/exif"
	"github.com/saidtezel/pascha/internal/sink"
	"github.com/saidtezel/pascha/internal/urlerr"
)

// Download streams the asset bytes from the signed CDN URL to a
// temporary file. The URL is short-lived; this transition must be
// called soon after the Discovered value was produced.
//
// Streaming-to-disk avoids holding entire video bytes in memory.
// SHA1 is computed during the copy.
func (d Discovered) Download(ctx context.Context, hc *http.Client) (Downloaded, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return Downloaded{}, fmt.Errorf("asset: build download request for %s: %w", d.famlyImageID, urlerr.Redact(err))
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Downloaded{}, fmt.Errorf("asset: download %s: %w", d.famlyImageID, urlerr.Redact(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Downloaded{}, fmt.Errorf("asset: download %s: HTTP %d", d.famlyImageID, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "pascha-dl-*"+filepath.Ext(d.filename))
	if err != nil {
		return Downloaded{}, fmt.Errorf("asset: create temp for %s: %w", d.famlyImageID, err)
	}
	hasher := sha1.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return Downloaded{}, fmt.Errorf("asset: stream body for %s: %w", d.famlyImageID, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return Downloaded{}, fmt.Errorf("asset: fsync temp for %s: %w", d.famlyImageID, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return Downloaded{}, fmt.Errorf("asset: close temp for %s: %w", d.famlyImageID, err)
	}
	return Downloaded{
		d:           d,
		tempPath:    tmp.Name(),
		sha1:        hex.EncodeToString(hasher.Sum(nil)),
		size:        written,
		contentType: resp.Header.Get("Content-Type"),
	}, nil
}

// Cleanup removes the temp file created by Download. Callers
// should defer this once Downloaded has been produced; pascha
// keeps no local copy, so the temp file goes once the asset is
// uploaded or has failed.
func (dl Downloaded) Cleanup() {
	if dl.tempPath != "" {
		_ = os.Remove(dl.tempPath)
	}
}

// Prepare readies the temp file for upload: it picks the filename
// extension from the reported Content-Type, reinjects EXIF into a
// JPEG in place, and builds the XMP sidecar for a video. A failed
// EXIF reinjection is not fatal: Prepared.ExifError carries it and
// the asset uploads with the metadata it arrived with.
func (dl Downloaded) Prepare(software string) (Prepared, error) {
	// Choose the file extension from the actual Content-Type the
	// CDN reported, so a vendor that ever serves HEIC or PNG
	// produces correctly-named files instead of misleading .jpg.
	filename := filenameForContentType(dl.d.filename, dl.contentType)
	sidecar, err := dl.d.Sidecar(software)
	if err != nil {
		return Prepared{}, fmt.Errorf("asset: sidecar %s: %w", dl.d.famlyImageID, err)
	}
	p := Prepared{dl: dl, filename: filename, sidecar: sidecar}
	if isJPEG(filename) {
		if err := exif.Reinject(dl.tempPath, dl.d.EXIF(software)); err != nil {
			p.exifError = err.Error()
		}
	}
	return p, nil
}

// Upload sends the prepared file to Immich.
func (p Prepared) Upload(ctx context.Context, immich *sink.Immich) (Uploaded, error) {
	if immich == nil {
		return Uploaded{}, ErrSinkNil
	}
	in := sink.PutInput{
		FamlyImageID:  p.dl.d.famlyImageID,
		Source:        string(p.dl.d.source),
		FeedItemID:    p.dl.d.feedItemID,
		SourcePath:    p.dl.tempPath,
		Filename:      p.filename,
		SHA1:          p.dl.sha1,
		FileCreatedAt: p.dl.d.fileCreatedAt,
		Sidecar:       p.sidecar,
	}
	receipt, err := immich.Put(ctx, in)
	if err != nil {
		return Uploaded{}, fmt.Errorf("asset: upload %s: %w", p.dl.d.famlyImageID, err)
	}
	return Uploaded{prepared: p, immichID: receipt.DestPath, status: receipt.Status}, nil
}

// ErrSinkNil is returned by transitions that require a sink that
// wasn't configured.
var ErrSinkNil = errors.New("asset: required sink is nil")

// filenameForContentType swaps the extension on filename to one
// that matches contentType. Falls back to the supplied filename
// unchanged when the type is unknown or empty.
func filenameForContentType(filename, contentType string) string {
	ext := extForContentType(contentType)
	if ext == "" {
		return filename
	}
	if old := pathExt(filename); strings.EqualFold(old, ext) {
		return filename
	}
	return strings.TrimSuffix(filename, pathExt(filename)) + ext
}

func pathExt(name string) string {
	for i := len(name) - 1; i >= 0 && name[i] != '/'; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

func extForContentType(ct string) string {
	if ct == "" {
		return ""
	}
	mime := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/heic", "image/heif":
		return ".heic"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/tiff":
		return ".tiff"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	}
	return ""
}

func isJPEG(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return true
	}
	return false
}
