package sink

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/saidtezel/pascha/api/immich"
)

// Immich is the destination sink and the record of what has been
// uploaded. Requires IMMICH_BASE_URL and IMMICH_API_KEY (with the
// asset.upload and asset.read permissions). Uploads the prepared
// temp file; nothing is kept on local disk once the upload is
// confirmed.
//
// pascha keeps no state of its own: every uploaded file name
// carries the Famly id, and Has finds it again by name.
type Immich struct {
	client          *immich.Client
	filenamePattern string
}

// idToken matches the {{.ID}} template action, spaces and trim
// markers allowed.
var idToken = regexp.MustCompile(`\{\{-?\s*\.ID\s*-?\}\}`)

// ErrPatternWithoutID is returned by NewImmich for a filename pattern
// that leaves out {{.ID}}: Has could not find those uploads again,
// so every run would upload them anew.
var ErrPatternWithoutID = errors.New("sink: filename pattern must contain {{.ID}}; pascha finds earlier uploads by the Famly id in their file name")

// NewImmich constructs an Immich sink. filenamePattern names the
// uploaded file (Immich's original file name) and must contain
// {{.ID}}; empty falls back to DefaultFilenamePattern.
func NewImmich(client *immich.Client, filenamePattern string) (*Immich, error) {
	if filenamePattern == "" {
		filenamePattern = DefaultFilenamePattern
	}
	if !idToken.MatchString(filenamePattern) {
		return nil, ErrPatternWithoutID
	}
	return &Immich{client: client, filenamePattern: filenamePattern}, nil
}

// Name implements Sink.
func (Immich) Name() string { return "immich" }

// Put reads the file at in.SourcePath and uploads it to Immich
// under the rendered filename, with the Famly identifiers
// preserved as Immich asset metadata.
//
// The duplicate-detection happens server-side via x-immich-checksum;
// the receipt's Status reflects what Immich did.
func (i *Immich) Put(ctx context.Context, in PutInput) (Receipt, error) {
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	name, err := renderPattern(i.filenamePattern, in)
	if err != nil {
		return Receipt{}, fmt.Errorf("sink: render filename pattern: %w", err)
	}
	if name == "" {
		return Receipt{}, fmt.Errorf("sink: filename pattern produced empty name")
	}
	data, err := os.ReadFile(in.SourcePath)
	if err != nil {
		return Receipt{}, fmt.Errorf("sink: read %s: %w", in.SourcePath, err)
	}
	res, err := i.client.Upload(ctx, immich.UploadInput{
		Data:           data,
		Filename:       name,
		Sidecar:        in.Sidecar,
		FileCreatedAt:  in.FileCreatedAt,
		FileModifiedAt: in.FileCreatedAt,
		Metadata: map[string]string{
			"famlyImageId":  in.FamlyImageID,
			"famlyFeedItem": in.FeedItemID,
			"famlySource":   in.Source,
		},
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("sink: upload %s: %w", in.FamlyImageID, err)
	}
	return Receipt{
		DestPath: res.ID,
		Status:   res.Status,
		Size:     int64(len(data)),
	}, nil
}

// Has reports whether Immich already holds the asset with this Famly
// id, trashed or not, and returns its Immich id. A hit is an asset
// whose original file name contains famlyID as a whole token, so a
// short id never matches inside a longer one.
func (i *Immich) Has(ctx context.Context, famlyID string) (string, bool, error) {
	if famlyID == "" {
		return "", false, errors.New("sink: Has with empty id")
	}
	assets, err := i.client.SearchByFilename(ctx, famlyID)
	if err != nil {
		return "", false, fmt.Errorf("sink: look up %s: %w", famlyID, err)
	}
	for _, a := range assets {
		if containsToken(a.OriginalFileName, famlyID) {
			return a.ID, true, nil
		}
	}
	return "", false, nil
}

// containsToken reports whether tok occurs in s, ignoring case, with
// no letter or digit directly before or after it.
func containsToken(s, tok string) bool {
	ls, lt := strings.ToLower(s), strings.ToLower(tok)
	for from := 0; ; {
		i := strings.Index(ls[from:], lt)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(lt)
		if !wordRuneBefore(ls, start) && !wordRuneAt(ls, end) {
			return true
		}
		from = start + 1
	}
}

func wordRuneBefore(s string, i int) bool {
	if i == 0 {
		return false
	}
	r := []rune(s[:i])
	return isWord(r[len(r)-1])
}

func wordRuneAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	return isWord([]rune(s[i:])[0])
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
