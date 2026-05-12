package commands

import (
	"context"
	"io"

	"github.com/Topline-com/os-cli/internal/output"
	"github.com/Topline-com/os-cli/internal/topline"
)

type setupProbe struct {
	Area  string `json:"area"`
	Path  string `json:"path"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func runSetupCheck(stdout io.Writer, globals globalOptions) error {
	cfg, err := topline.LoadConfig()
	if err != nil {
		return err
	}
	if globals.BaseURL != "" {
		cfg.BaseURL = globals.BaseURL
	}
	if globals.LocationID != "" {
		cfg.LocationID = globals.LocationID
	}
	client := topline.NewClient(cfg)
	probes := []setupProbe{
		{Area: "location", Path: "/locations/" + cfg.LocationID},
		{Area: "contacts", Path: "/contacts/"},
		{Area: "conversations", Path: "/conversations/search"},
		{Area: "opportunities", Path: "/opportunities/pipelines"},
		{Area: "calendars", Path: "/calendars/"},
		{Area: "workflows", Path: "/workflows/"},
		{Area: "forms", Path: "/forms/"},
		{Area: "surveys", Path: "/surveys/"},
		{Area: "users", Path: "/users/"},
		{Area: "custom_fields", Path: "/locations/" + cfg.LocationID + "/customFields"},
		{Area: "tags", Path: "/locations/" + cfg.LocationID + "/tags"},
	}
	ok := 0
	for i := range probes {
		query := map[string]string{"locationId": cfg.LocationID, "limit": "1"}
		if probes[i].Area == "opportunities" || probes[i].Area == "custom_fields" || probes[i].Area == "tags" || probes[i].Area == "location" {
			query = map[string]string{"locationId": cfg.LocationID}
		}
		var result any
		err := client.Do(context.Background(), topline.Request{Method: "GET", Path: probes[i].Path, Query: query}, &result)
		if err == nil {
			probes[i].OK = true
			ok++
		} else {
			probes[i].Error = err.Error()
		}
	}
	summary := "Setup incomplete."
	if ok == len(probes) {
		summary = "All scope areas OK. Topline OS CLI is fully set up."
	}
	return output.WriteJSON(stdout, map[string]any{
		"brand":    cfg.BrandName,
		"auth":     map[string]any{"ok": true, "tokenPrefix": topline.MaskToken(cfg.PIT)},
		"location": map[string]any{"ok": true, "id": cfg.LocationID},
		"scopes":   probes,
		"summary":  summary,
	}, globals.MaskPII)
}
