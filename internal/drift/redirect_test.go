package drift

import (
	"testing"

	"github.com/saidtezel/pascha/internal/safehttp/safehttptest"
)

func TestDefaultClientRedirectPolicy(t *testing.T) {
	safehttptest.AssertPolicy(t, defaultClient())
}
