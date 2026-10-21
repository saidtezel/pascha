package sink

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// DefaultFilenamePattern is jacobbunk-style. Source values like
// "feed-image" already carry the type prefix, so a literal extra
// "feed-" would double up.
const DefaultFilenamePattern = "{{.Source}}-%Y-%m-%d_%H-%M-%S-{{.ID}}.{{.Ext}}"

// renderPattern expands a pattern string in two passes: strftime
// tokens (%Y, %m, %d, %H, %M, %S, %j) get replaced with the
// FileCreatedAt date components first; then the result is run
// through text/template against PutInput-derived fields.
func renderPattern(pattern string, in PutInput) (string, error) {
	stage1 := expandStrftime(pattern, in.FileCreatedAt)
	tmpl, err := template.New("pattern").Option("missingkey=error").Parse(stage1)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	ctx := struct {
		Source     string
		ID         string
		FeedItemID string
		Ext        string
	}{
		Source:     in.Source,
		ID:         in.FamlyImageID,
		FeedItemID: in.FeedItemID,
		Ext:        strings.TrimPrefix(filepath.Ext(in.Filename), "."),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}
	return buf.String(), nil
}

// expandStrftime handles a small subset of strftime tokens. We
// avoid pulling in a strftime library; the supported tokens are
// the ones pascha's templating actually needs.
func expandStrftime(pattern string, when time.Time) string {
	// Use UTC to keep filenames stable across timezones; the
	// timestamp source is UTC anyway.
	when = when.UTC()
	r := strings.NewReplacer(
		"%Y", when.Format("2006"),
		"%m", when.Format("01"),
		"%d", when.Format("02"),
		"%H", when.Format("15"),
		"%M", when.Format("04"),
		"%S", when.Format("05"),
		"%j", when.Format("002"),
	)
	return r.Replace(pattern)
}
