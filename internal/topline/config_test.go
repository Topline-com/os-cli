package topline

import (
	"testing"
)

func TestLoadConfigReadsEnvAndMasksToken(t *testing.T) {
	t.Setenv("TOPLINE_PIT", "pit-abcdef1234567890")
	t.Setenv("TOPLINE_LOCATION_ID", "loc_123")
	t.Setenv("TOPLINE_BRAND_NAME", "Topline OS")
	t.Setenv("TOPLINE_BASE_URL", "https://example.test")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.PIT != "pit-abcdef1234567890" {
		t.Fatalf("PIT mismatch: %q", cfg.PIT)
	}
	if cfg.LocationID != "loc_123" {
		t.Fatalf("LocationID mismatch: %q", cfg.LocationID)
	}
	if cfg.BrandName != "Topline OS" {
		t.Fatalf("BrandName mismatch: %q", cfg.BrandName)
	}
	if cfg.BaseURL != "https://example.test" {
		t.Fatalf("BaseURL mismatch: %q", cfg.BaseURL)
	}
	if got := MaskToken(cfg.PIT); got != "pit-ab…7890" {
		t.Fatalf("masked token mismatch: %q", got)
	}
}

func TestLoadConfigRejectsMissingAuth(t *testing.T) {
	t.Setenv("TOPLINE_PIT", "")
	t.Setenv("TOPLINE_LOCATION_ID", "loc_123")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected missing PIT error")
	}
}
