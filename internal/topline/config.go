package topline

import (
	"errors"
	"os"
	"strings"
)

const (
	DefaultBaseURL = "https://services.leadconnectorhq.com"
	DefaultBrand   = "Topline OS"
	APIVersion     = "2021-07-28"
)

type Config struct {
	PIT        string
	LocationID string
	BrandName  string
	BaseURL    string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		PIT:        strings.TrimSpace(os.Getenv("TOPLINE_PIT")),
		LocationID: strings.TrimSpace(os.Getenv("TOPLINE_LOCATION_ID")),
		BrandName:  strings.TrimSpace(os.Getenv("TOPLINE_BRAND_NAME")),
		BaseURL:    strings.TrimSpace(os.Getenv("TOPLINE_BASE_URL")),
	}
	if cfg.BrandName == "" {
		cfg.BrandName = DefaultBrand
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.PIT == "" {
		return cfg, errors.New("TOPLINE_PIT is required")
	}
	if cfg.LocationID == "" {
		return cfg, errors.New("TOPLINE_LOCATION_ID is required")
	}
	return cfg, nil
}

func MaskToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 10 {
		if len(token) <= 4 {
			return token + "…"
		}
		return token[:4] + "…"
	}
	return token[:6] + "…" + token[len(token)-4:]
}
