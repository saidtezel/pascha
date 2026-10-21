package config

import "testing"

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
