package asset

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/saidtezel/pascha/api/famly"
	"github.com/saidtezel/pascha/api/immich"
	"github.com/saidtezel/pascha/internal/sink"
)

func TestFilenameForContentType(t *testing.T) {
	cases := []struct {
		filename, ct, want string
	}{
		{"img.jpg", "image/jpeg", "img.jpg"},
		{"img.jpg", "image/png", "img.png"},
		{"img.jpg", "image/heic", "img.heic"},
		{"img.jpg", "image/heif", "img.heic"}, // Apple convention: .heic for both
		{"img.jpg", "image/webp", "img.webp"},
		{"img.jpg", "video/mp4", "img.mp4"},
		{"img.jpg", "image/jpeg; charset=binary", "img.jpg"},
		{"img.jpg", "", "img.jpg"},
		{"img.jpg", "application/octet-stream", "img.jpg"},
		{"abc.heic", "image/jpeg", "abc.jpg"},
	}
	for _, c := range cases {
		got := filenameForContentType(c.filename, c.ct)
		if got != c.want {
			t.Errorf("filenameForContentType(%q, %q) = %q, want %q",
				c.filename, c.ct, got, c.want)
		}
	}
}

func TestDiscoverImagePicksTime(t *testing.T) {
	created := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	img := famly.Image{
		ImageID:   "img-001",
		URL:       "https://fixture/image/img-001",
		BigURL:    "https://fixture/image/img-001/big",
		CreatedAt: famly.ImageTime{Date: famly.FamlyTime{Time: created.Add(time.Minute)}},
	}
	item := famly.FeedItem{
		FeedItemID:  "post-001",
		Body:        "captioned",
		CreatedDate: famly.FamlyTime{Time: created},
		Sender:      &famly.Sender{Name: "Educator A"},
	}
	d := DiscoverImage(img, item, time.UTC)
	if d.FamlyImageID() != "img-001" {
		t.Errorf("FamlyImageID = %q", d.FamlyImageID())
	}
	if d.URL() != "https://fixture/image/img-001/big" {
		t.Errorf("URL should prefer big variant, got %q", d.URL())
	}
	if !d.FileCreatedAt().Equal(created.Add(time.Minute)) {
		t.Errorf("FileCreatedAt = %v", d.FileCreatedAt())
	}
	if d.SenderName() != "Educator A" {
		t.Errorf("SenderName = %q", d.SenderName())
	}

	// EXIF derivation
	ef := d.EXIF("pascha 0.1")
	if !ef.DateTimeOriginal.Equal(created.Add(time.Minute)) {
		t.Errorf("EXIF.DateTimeOriginal = %v", ef.DateTimeOriginal)
	}
	if ef.Artist != "Educator A" {
		t.Errorf("EXIF.Artist = %q", ef.Artist)
	}
	if ef.UserComment != "captioned - Educator A" {
		t.Errorf("EXIF.UserComment = %q", ef.UserComment)
	}
	if ef.Software != "pascha 0.1" {
		t.Errorf("EXIF.Software = %q", ef.Software)
	}
}

// makeJPEG returns the bytes of a tiny solid-colour JPEG.
func makeJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{0xff, 0x80, 0x40, 0xff})
		}
	}
	f, _ := os.CreateTemp("", "fixture-*.jpg")
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	body, _ := os.ReadFile(f.Name())
	_ = os.Remove(f.Name())
	return body
}

func TestPipelineUploads(t *testing.T) {
	jpegBytes := makeJPEG(t)

	// Famly side serves the JPEG; Immich side records the upload.
	var uploadedName string
	var uploadedBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/image/img-001/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBytes)
	})
	mux.HandleFunc("/assets", func(w http.ResponseWriter, r *http.Request) {
		f, hdr, err := r.FormFile("assetData")
		if err == nil {
			uploadedName = hdr.Filename
			uploadedBody, _ = io.ReadAll(f)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"asset-1","status":"created"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	created := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	img := famly.Image{
		ImageID:   "img-001",
		BigURL:    srv.URL + "/image/img-001/big",
		CreatedAt: famly.ImageTime{Date: famly.FamlyTime{Time: created}},
	}
	item := famly.FeedItem{
		FeedItemID:  "post-001",
		Body:        "captioned",
		CreatedDate: famly.FamlyTime{Time: created},
		Sender:      &famly.Sender{Name: "Educator A"},
	}

	disc := DiscoverImage(img, item, time.UTC)

	dl, err := disc.Download(context.Background(), nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer dl.Cleanup()
	if dl.Size() == 0 {
		t.Errorf("Size = 0")
	}

	prep, err := dl.Prepare("pascha 0.1-test")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prep.ExifError() != "" {
		t.Errorf("ExifError = %q", prep.ExifError())
	}

	im, err := sink.NewImmich(immich.New(srv.URL, "k"), "")
	if err != nil {
		t.Fatal(err)
	}
	up, err := prep.Upload(context.Background(), im)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if up.ImmichAssetID() != "asset-1" {
		t.Errorf("ImmichAssetID = %q", up.ImmichAssetID())
	}
	if uploadedName != "feed-image-2026-05-06_14-00-00-img-001.jpg" {
		t.Errorf("uploaded filename = %q", uploadedName)
	}
	// The uploaded bytes carry the reinjected caption.
	if !bytes.Contains(uploadedBody, []byte("captioned")) {
		t.Error("uploaded JPEG lacks the reinjected EXIF caption")
	}

	// No local copy: the temp file goes on Cleanup.
	dl.Cleanup()
	if _, err := os.Stat(dl.TempPath()); !os.IsNotExist(err) {
		t.Errorf("temp file still present after Cleanup: %v", err)
	}
}
