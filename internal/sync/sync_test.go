package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Topline-com/os-cli/internal/topline"
)

func TestSyncAll_OpportunitiesAndPipelines(t *testing.T) {
	pipelinesBody := map[string]any{
		"pipelines": []any{
			map[string]any{
				"id":   "PIPE1",
				"name": "Sales - Upwork",
				"stages": []any{
					map[string]any{"id": "STG1", "name": "Proposal Pending"},
					map[string]any{"id": "STG2", "name": "Proposal Sent"},
				},
			},
		},
	}
	page1 := map[string]any{
		"opportunities": []any{
			map[string]any{
				"id":              "OPP1",
				"name":            "Acme",
				"status":          "open",
				"pipelineId":      "PIPE1",
				"pipelineStageId": "STG1",
				"contactId":       "C1",
				"monetaryValue":   1000.0,
				"updatedAt":       "2026-05-10T00:00:00Z",
			},
		},
		"meta": map[string]any{"startAfterId": "OPP1", "startAfter": "1"},
	}
	page2 := map[string]any{
		"opportunities": []any{
			map[string]any{
				"id":              "OPP2",
				"name":            "Initech",
				"status":          "open",
				"pipelineId":      "PIPE1",
				"pipelineStageId": "STG2",
				"contact":         map[string]any{"id": "C2"},
				"monetaryValue":   2500.0,
				"updatedAt":       "2026-05-12T00:00:00Z",
			},
		},
	}
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/opportunities/pipelines":
			_ = json.NewEncoder(w).Encode(pipelinesBody)
		case "/opportunities/search":
			calls++
			if r.URL.Query().Get("startAfterId") == "" {
				_ = json.NewEncoder(w).Encode(page1)
			} else {
				_ = json.NewEncoder(w).Encode(page2)
			}
		case "/contacts/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []any{}})
		case "/conversations/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := topline.NewClient(topline.Config{BaseURL: ts.URL, PIT: "test", LocationID: "LOC"})
	dbPath := filepath.Join(t.TempDir(), "t.db")
	res, err := SyncAll(context.Background(), client, "LOC", dbPath)
	if err != nil {
		t.Fatalf("SyncAll: %v", err)
	}
	// SyncAll runs pipelines + opportunities + contacts + conversations +
	// messages. This test asserts only on pipelines/opportunities; the rest
	// of the entity coverage lives in sync_entities_test.go.
	if len(res.Results) < 2 {
		t.Fatalf("expected at least 2 entity results, got %d", len(res.Results))
	}
	var oppRes Result
	for _, r := range res.Results {
		if r.Entity == "opportunities" {
			oppRes = r
		}
	}
	if oppRes.Upserted != 2 {
		t.Fatalf("expected 2 opportunities upserted, got %d", oppRes.Upserted)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&n); err != nil {
		t.Fatalf("count opps: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows in opportunities, got %d", n)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM pipelines WHERE id='PIPE1'`).Scan(&name); err != nil {
		t.Fatalf("pipeline name: %v", err)
	}
	if name != "Sales - Upwork" {
		t.Fatalf("unexpected pipeline name %q", name)
	}
	var stages int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pipeline_stages WHERE pipeline_id='PIPE1'`).Scan(&stages); err != nil {
		t.Fatalf("stage count: %v", err)
	}
	if stages != 2 {
		t.Fatalf("expected 2 stages, got %d", stages)
	}
	// Idempotency: re-run should not duplicate.
	if _, err := SyncAll(context.Background(), client, "LOC", dbPath); err != nil {
		t.Fatalf("second SyncAll: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM opportunities`).Scan(&n); err != nil {
		t.Fatalf("count opps 2: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected idempotent count 2, got %d", n)
	}
}
