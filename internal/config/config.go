// Package config loads pascha's configuration from flags, env vars,
// and an optional TOML file. Precedence: flags > env > file > zero.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/saidtezel/pascha/api/famly"
)

// Config is the application-level configuration. Populated by Load.
type Config struct {
	// FamlyAccessToken is the session token (preferred for short
	// runs and CI).
	FamlyAccessToken string

	// FamlyEmail and FamlyPassword enable token refresh on 401.
	FamlyEmail    string
	FamlyPassword string

	// FamlyDeviceID is a stable per-host identifier sent to Famly's
	// Authenticate mutation. Optional; falls back to a constant.
	FamlyDeviceID string

	// FamlyBaseURL overrides the production endpoint. Tests only.
	FamlyBaseURL string

	// ImmichBaseURL is the Immich API root, e.g.
	// "https://immich.home.example/api". Required for fetch.
	ImmichBaseURL string

	// ImmichAPIKey is the x-api-key header value. Required for fetch.
	ImmichAPIKey string

	// Zone is the IANA name of the fallback zone for videos in posts
	// with no zoned image (PASCHA_TZ). Empty means the machine's
	// local zone.
	Zone string

	// LogFormat is "json" (cron-friendly) or "text" (interactive).
	LogFormat string

	// Interval, when non-zero, keeps fetch running and starts a new
	// fetch this long after the previous one began (PASCHA_INTERVAL,
	// a Go duration such as "6h"). Zero means fetch once and exit.
	Interval time.Duration
}

// Load reads config from environment variables, applying defaults
// where unset. Flag values from the caller win and may be merged
// in afterwards.
func Load() (*Config, error) {
	c := &Config{
		FamlyAccessToken: os.Getenv("FAMLY_ACCESS_TOKEN"),
		FamlyEmail:       os.Getenv("FAMLY_EMAIL"),
		FamlyPassword:    os.Getenv("FAMLY_PASSWORD"),
		FamlyDeviceID:    os.Getenv("FAMLY_DEVICE_ID"),
		FamlyBaseURL:     os.Getenv("FAMLY_BASE_URL"),
		ImmichBaseURL:    os.Getenv("IMMICH_BASE_URL"),
		ImmichAPIKey:     os.Getenv("IMMICH_API_KEY"),
		Zone:             os.Getenv("PASCHA_TZ"),
		LogFormat:        os.Getenv("PASCHA_LOG_FORMAT"),
	}
	if c.FamlyDeviceID == "" {
		// Famly's DeviceId scalar validates as UUID. Use a stable
		// per-host derivation rather than a constant string.
		c.FamlyDeviceID = famly.DeriveDeviceID()
	}
	if c.LogFormat == "" {
		c.LogFormat = "json"
	}
	if v := os.Getenv("PASCHA_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("PASCHA_INTERVAL: %w", err)
		}
		c.Interval = d
	}
	return c, nil
}

// Validate checks that required fields are set for the given run
// mode. fetch requires Famly auth and Immich: pascha uploads
// straight to Immich and keeps no local archive.
func (c *Config) Validate(mode string) error {
	switch mode {
	case "fetch":
		if c.FamlyAccessToken == "" && (c.FamlyEmail == "" || c.FamlyPassword == "") {
			return errors.New("config: set FAMLY_EMAIL and FAMLY_PASSWORD (recommended) or FAMLY_ACCESS_TOKEN; run \"pascha login\" to verify credentials")
		}
		if c.ImmichBaseURL == "" || c.ImmichAPIKey == "" {
			return errors.New("config: fetch uploads to Immich; set IMMICH_BASE_URL and IMMICH_API_KEY")
		}
	case "drift":
		// drift is multi-vendor (Famly + Immich). Per-vendor auth
		// checks happen at runDrift() once the manifest is loaded:
		// Famly's auth_env triggers the refreshing-token path,
		// other vendors flow through Probe's env-fallback.
	}
	return nil
}
