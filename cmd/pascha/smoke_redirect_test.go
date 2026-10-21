package main

import (
	"testing"

	"github.com/saidtezel/pascha/internal/safehttp/safehttptest"
)

func TestSmokeHTTPClientRedirectPolicy(t *testing.T) {
	safehttptest.AssertPolicy(t, smokeHTTPClient())
}
