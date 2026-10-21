package sink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saidtezel/pascha/api/immich"
)

func TestNewImmichRequiresIDInPattern(t *testing.T) {
	c := immich.New("http://127.0.0.1:1", "k")
	for _, p := range []string{"", DefaultFilenamePattern, "{{ .ID }}.{{.Ext}}", "x-{{- .ID -}}.jpg"} {
		if _, err := NewImmich(c, p); err != nil {
			t.Errorf("NewImmich(%q) = %v, want ok", p, err)
		}
	}
	if _, err := NewImmich(c, "{{.Source}}-%Y.{{.Ext}}"); !errors.Is(err, ErrPatternWithoutID) {
		t.Errorf("pattern without ID: err = %v, want ErrPatternWithoutID", err)
	}
}

func TestContainsToken(t *testing.T) {
	cases := []struct {
		s, tok string
		want   bool
	}{
		{"feed-image-2026-05-06_14-00-00-img-1.jpg", "img-1", true},
		{"feed-image-2026-05-06_14-00-00-img-10.jpg", "img-1", false},
		{"feed-image-2026-05-06_14-00-00-ximg-1.jpg", "img-1", false},
		{"FEED-IMAGE-ABC-DEF.JPG", "abc-def", true},
		{"img-10-img-1.jpg", "img-1", true}, // second occurrence is whole
		{"other.jpg", "img-1", false},
	}
	for _, c := range cases {
		if got := containsToken(c.s, c.tok); got != c.want {
			t.Errorf("containsToken(%q, %q) = %v, want %v", c.s, c.tok, got, c.want)
		}
	}
}

func TestHasMatchesWholeIDOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"assets":{"items":[{"id":"a10","originalFileName":"feed-image-2026-05-06_14-00-00-img-10.jpg"}]}}`)
	}))
	t.Cleanup(srv.Close)
	im, err := NewImmich(immich.New(srv.URL, "k"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := im.Has(context.Background(), "img-1"); err != nil || ok {
		t.Errorf("Has(img-1) = %v, %v; want not found", ok, err)
	}
	if id, ok, err := im.Has(context.Background(), "img-10"); err != nil || !ok || id != "a10" {
		t.Errorf("Has(img-10) = %q, %v, %v; want a10", id, ok, err)
	}
}
