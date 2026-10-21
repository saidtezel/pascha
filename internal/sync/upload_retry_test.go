package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/saidtezel/pascha/api/famly"
)

func runWith(t *testing.T, famlySrv *httptest.Server, im *fakeImmich) Result {
	t.Helper()
	fc := famly.New(famly.NewStaticToken("t"), famly.WithBaseURL(famlySrv.URL), famly.WithPageDelay(0))
	res, err := Run(context.Background(), Deps{Famly: fc, Immich: im.sink(t)},
		Options{MaxPages: 2, Source: SourceAll})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

func TestFailedUploadIsRetriedOnRerun(t *testing.T) {
	famlySrv := fakeFamlyServer(t)
	t.Cleanup(famlySrv.Close)
	im := newFakeImmich(t, "fail", "created")

	r1 := runWith(t, famlySrv, im)
	if r1.UploadFailed != 1 || r1.Uploaded != 0 {
		t.Fatalf("run1 failed=%d uploaded=%d, want 1/0", r1.UploadFailed, r1.Uploaded)
	}

	r2 := runWith(t, famlySrv, im)
	if r2.Uploaded != 1 || r2.UploadFailed != 0 {
		t.Fatalf("run2 uploaded=%d failed=%d, want 1/0", r2.Uploaded, r2.UploadFailed)
	}

	// The retry carries the same timeline date as the first attempt:
	// the post's createdDate, not the retry's clock.
	const wantDate = "@2026-05-06T14:00:00Z"
	if len(im.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(im.requests))
	}
	for i, b := range im.requests {
		if !strings.HasSuffix(b, wantDate) {
			t.Errorf("request %d = %q, want fileCreatedAt %s", i, b, wantDate)
		}
	}

	r3 := runWith(t, famlySrv, im)
	if r3.Uploaded != 0 || r3.Skipped != 1 || im.uploads.Load() != 2 {
		t.Fatalf("run3 uploaded=%d skipped=%d uploads=%d, want 0/1/2", r3.Uploaded, r3.Skipped, im.uploads.Load())
	}
}

func TestDuplicateUploadResponseCountsAsConfirmed(t *testing.T) {
	famlySrv := fakeFamlyServer(t)
	t.Cleanup(famlySrv.Close)
	im := newFakeImmich(t, "duplicate")

	r := runWith(t, famlySrv, im)
	if r.UploadDuplicates != 1 || r.Uploaded != 0 || r.UploadFailed != 0 {
		t.Fatalf("dup=%d uploaded=%d failed=%d, want 1/0/0", r.UploadDuplicates, r.Uploaded, r.UploadFailed)
	}
}

// An asset Immich already holds under its pascha file name (from an
// earlier run, another container, or a bairn upload) is skipped
// before any download, whatever the rest of the name says.
func TestAssetAlreadyInImmichIsNotDownloaded(t *testing.T) {
	famlySrv, downloads := manyImagesFamly(t, 2)
	im := newFakeImmich(t)
	im.have("feed-image-2020-01-01_00-00-00-img-0.jpg")

	r := runWith(t, famlySrv, im)
	if r.Skipped != 1 || r.Uploaded != 1 {
		t.Fatalf("skipped=%d uploaded=%d, want 1/1", r.Skipped, r.Uploaded)
	}
	if downloads.Load() != 1 {
		t.Fatalf("downloads = %d, want 1 (img-1 only)", downloads.Load())
	}
}

func TestDryRunChecksImmichButDoesNotUpload(t *testing.T) {
	famlySrv, downloads := manyImagesFamly(t, 2)
	im := newFakeImmich(t)
	im.have("feed-image-2020-01-01_00-00-00-img-0.jpg")

	fc := famly.New(famly.NewStaticToken("t"), famly.WithBaseURL(famlySrv.URL), famly.WithPageDelay(0))
	res, err := Run(context.Background(), Deps{Famly: fc, Immich: im.sink(t)},
		Options{MaxPages: 2, Source: SourceAll, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if im.uploads.Load() != 0 || downloads.Load() != 0 {
		t.Fatalf("dry run uploads=%d downloads=%d, want 0/0", im.uploads.Load(), downloads.Load())
	}
	if res.Skipped != 1 || im.searches.Load() != 2 {
		t.Fatalf("skipped=%d searches=%d, want 1/2", res.Skipped, im.searches.Load())
	}
}

func TestLookupFailureSkipsAssetAndStopsRun(t *testing.T) {
	famlySrv, downloads := manyImagesFamly(t, 8)
	im := newFakeImmich(t)
	im.searchFail.Store(true)

	r := runWith(t, famlySrv, im)
	if r.UploadFailed != 8 || r.Uploaded != 0 {
		t.Fatalf("failed=%d uploaded=%d, want 8/0", r.UploadFailed, r.Uploaded)
	}
	if im.searches.Load() != maxUploadFailures || downloads.Load() != 0 {
		t.Fatalf("searches=%d downloads=%d, want %d/0", im.searches.Load(), downloads.Load(), maxUploadFailures)
	}
}

func TestVideoRetryResendsSidecar(t *testing.T) {
	mux := http.NewServeMux()
	famlySrv := httptest.NewServer(mux)
	t.Cleanup(famlySrv.Close)
	mux.HandleFunc("/api/feed/feed/feed", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "" {
			_, _ = w.Write([]byte(`{"feedItems": []}`))
			return
		}
		fmt.Fprintf(w, `{"feedItems":[{"feedItemId":"p","originatorId":"Post:e",
"createdDate":"2026-05-06T14:00:00Z","body":"clip","images":[],
"videos":[{"videoId":"vid-1","videoUrl":"%s/v/1"}]}]}`, famlySrv.URL)
	})
	mux.HandleFunc("/v/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("not really a video"))
	})
	im := newFakeImmich(t, "fail", "created")

	runWith(t, famlySrv, im)
	r := runWith(t, famlySrv, im)
	if r.Uploaded != 1 {
		t.Fatalf("uploaded=%d, want 1", r.Uploaded)
	}
	if len(im.requests) != 2 || !strings.Contains(im.requests[1], "sidecarData") {
		t.Fatalf("retry upload parts = %v, want sidecarData present", im.requests)
	}
}

// manyImagesFamly serves one page of n images, then an empty page.
// downloads counts the image fetches.
func manyImagesFamly(t *testing.T, n int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var downloads atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	body := jpegBytes(t)
	mux.HandleFunc("/api/feed/feed/feed", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "" {
			_, _ = w.Write([]byte(`{"feedItems": []}`))
			return
		}
		var imgs []string
		for i := 0; i < n; i++ {
			imgs = append(imgs, fmt.Sprintf(`{"imageId":"img-%d","url":"%s/img/%d","createdAt":{"date":"2026-05-06T14:00:00Z"}}`, i, srv.URL, i))
		}
		fmt.Fprintf(w, `{"feedItems":[{"feedItemId":"p","originatorId":"Post:e",
"createdDate":"2026-05-06T14:00:00Z","body":"b","images":[%s]}]}`, strings.Join(imgs, ","))
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	})
	return srv, &downloads
}

func TestUploadsStopAfterConsecutiveFailures(t *testing.T) {
	famlySrv, downloads := manyImagesFamly(t, 8)
	im := newFakeImmich(t, "fail", "fail", "fail", "fail", "fail", "created")

	r1 := runWith(t, famlySrv, im)
	if r1.UploadFailed != 8 || r1.Uploaded != 0 {
		t.Fatalf("run1 failed=%d uploaded=%d, want 8/0", r1.UploadFailed, r1.Uploaded)
	}
	if im.uploads.Load() != maxUploadFailures {
		t.Fatalf("immich uploads = %d, want %d", im.uploads.Load(), maxUploadFailures)
	}
	// Once uploads stop, the rest are not downloaded either.
	if downloads.Load() != maxUploadFailures {
		t.Fatalf("downloads = %d, want %d", downloads.Load(), maxUploadFailures)
	}

	// Immich holds none of them; a rerun uploads all 8.
	r2 := runWith(t, famlySrv, im)
	if r2.Uploaded != 8 || r2.UploadFailed != 0 {
		t.Fatalf("run2 uploaded=%d failed=%d, want 8/0", r2.Uploaded, r2.UploadFailed)
	}
}

func TestUploadsStopAtOnceOnForbidden(t *testing.T) {
	famlySrv, _ := manyImagesFamly(t, 4)
	im := newFakeImmich(t, "forbid")

	r := runWith(t, famlySrv, im)
	if im.uploads.Load() != 1 || r.UploadFailed != 4 {
		t.Fatalf("uploads=%d failed=%d, want 1/4", im.uploads.Load(), r.UploadFailed)
	}
}
