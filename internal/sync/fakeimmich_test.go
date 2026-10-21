package sync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	gosync "sync"
	"sync/atomic"
	"testing"

	"github.com/saidtezel/pascha/api/immich"
	"github.com/saidtezel/pascha/internal/retry"
	"github.com/saidtezel/pascha/internal/sink"
)

// fakeImmich is an in-memory Immich: /assets answers the next entry
// of script ("created", "duplicate", "fail" for HTTP 400, never
// retried by the client, or "forbid" for 403), repeating the last,
// and keeps the file name of every confirmed upload; /search/metadata
// finds those names by substring, as the real server does.
type fakeImmich struct {
	srv      *httptest.Server
	script   []string
	uploads  atomic.Int32 // /assets requests
	searches atomic.Int32 // /search/metadata requests
	// searchFail makes /search/metadata answer HTTP 500.
	searchFail atomic.Bool

	mu    gosync.Mutex
	names []string // confirmed uploads, plus any seeded with have
	// requests records each upload's file parts and fileCreatedAt.
	requests []string
}

func newFakeImmich(t *testing.T, script ...string) *fakeImmich {
	t.Helper()
	if len(script) == 0 {
		script = []string{"created"}
	}
	f := &fakeImmich{script: script}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets", f.upload)
	mux.HandleFunc("/search/metadata", f.search)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// have seeds an asset Immich already holds under this file name.
func (f *fakeImmich) have(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names = append(f.names, name)
}

func (f *fakeImmich) upload(w http.ResponseWriter, r *http.Request) {
	i := int(f.uploads.Add(1)) - 1
	if i >= len(f.script) {
		i = len(f.script) - 1
	}
	_ = r.ParseMultipartForm(1 << 20)
	var parts []string
	name := ""
	for k, fhs := range r.MultipartForm.File {
		parts = append(parts, k)
		if k == "assetData" && len(fhs) > 0 {
			name = fhs[0].Filename
		}
	}
	f.mu.Lock()
	f.requests = append(f.requests, strings.Join(parts, ",")+" @"+r.FormValue("fileCreatedAt"))
	f.mu.Unlock()
	switch f.script[i] {
	case "forbid":
		http.Error(w, "no", http.StatusForbidden)
		return
	case "fail":
		http.Error(w, "no", http.StatusBadRequest)
		return
	}
	f.have(name)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":"asset-%d","status":%q}`, i, f.script[i])
}

func (f *fakeImmich) search(w http.ResponseWriter, r *http.Request) {
	f.searches.Add(1)
	if f.searchFail.Load() {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	var q struct {
		OriginalFileName string `json:"originalFileName"`
	}
	_ = json.NewDecoder(r.Body).Decode(&q)
	f.mu.Lock()
	var items []map[string]string
	for i, n := range f.names {
		if strings.Contains(strings.ToLower(n), strings.ToLower(q.OriginalFileName)) {
			items = append(items, map[string]string{"id": fmt.Sprintf("held-%d", i), "originalFileName": n})
		}
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"assets": map[string]any{"items": items}})
}

func (f *fakeImmich) sink(t *testing.T) *sink.Immich {
	t.Helper()
	return newSink(t, f.srv.URL)
}

func newSink(t *testing.T, url string) *sink.Immich {
	t.Helper()
	// No retries: a scripted failure must reach the loop as one.
	im, err := sink.NewImmich(immich.New(url, "k", immich.WithRetry(noRetry())), "")
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func noRetry() retry.Policy {
	p := retry.Default()
	p.Attempts = 1
	return p
}
