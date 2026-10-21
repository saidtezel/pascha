package config

import (
	"testing"
	"time"
)

func TestValidateFetchRequiresImmich(t *testing.T) {
	famly := Config{FamlyEmail: "a@example.com", FamlyPassword: "p"}
	for _, c := range []struct {
		name    string
		url     string
		key     string
		wantErr bool
	}{
		{"both unset", "", "", true},
		{"url only", "https://immich.example/api", "", true},
		{"key only", "", "k", true},
		{"both set", "https://immich.example/api", "k", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := famly
			cfg.ImmichBaseURL, cfg.ImmichAPIKey = c.url, c.key
			if err := cfg.Validate("fetch"); (err != nil) != c.wantErr {
				t.Errorf("Validate = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestLoadInterval(t *testing.T) {
	for _, c := range []struct {
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"6h", 6 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"6", 0, true},
		{"soon", 0, true},
	} {
		t.Run(c.env, func(t *testing.T) {
			t.Setenv("PASCHA_INTERVAL", c.env)
			cfg, err := Load()
			if (err != nil) != c.wantErr {
				t.Fatalf("Load err = %v, wantErr %v", err, c.wantErr)
			}
			if err == nil && cfg.Interval != c.want {
				t.Errorf("Interval = %v, want %v", cfg.Interval, c.want)
			}
		})
	}
}
