package sink

import (
	"testing"
	"time"
)

func TestRenderPattern(t *testing.T) {
	when := time.Date(2026, 5, 6, 14, 30, 45, 0, time.UTC)
	got, err := renderPattern(DefaultFilenamePattern, PutInput{
		FamlyImageID:  "abc-123",
		Source:        "feed-image",
		Filename:      "abc-123.jpg",
		FileCreatedAt: when,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "feed-image-2026-05-06_14-30-45-abc-123.jpg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderPatternMissingKeyErrors(t *testing.T) {
	_, err := renderPattern("{{.NotAField}}-%Y", PutInput{FileCreatedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error for unknown template key")
	}
}
