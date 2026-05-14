package commands

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	localsync "github.com/Topline-com/os-cli/internal/sync"
	"github.com/Topline-com/os-cli/internal/topline"
)

func TestLocalCommands_EndToEnd(t *testing.T) {
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
	staleAt := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	freshAt := time.Now().UTC().Format(time.RFC3339)
	page := map[string]any{
		"opportunities": []any{
			map[string]any{
				"id":              "OPP_STALE",
				"name":            "Stale Acme",
				"status":          "open",
				"pipelineId":      "PIPE1",
				"pipelineStageId": "STG1",
				"contactId":       "C1",
				"monetaryValue":   1000.0,
				"updatedAt":       staleAt,
			},
			map[string]any{
				"id":              "OPP_FRESH",
				"name":            "Fresh Initech",
				"status":          "open",
				"pipelineId":      "PIPE1",
				"pipelineStageId": "STG2",
				"contactId":       "C2",
				"monetaryValue":   2500.0,
				"updatedAt":       freshAt,
			},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/opportunities/pipelines":
			_ = json.NewEncoder(w).Encode(pipelinesBody)
		case "/opportunities/search":
			_ = json.NewEncoder(w).Encode(page)
		case "/contacts/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []any{}})
		case "/conversations/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	client := topline.NewClient(topline.Config{BaseURL: ts.URL, PIT: "test", LocationID: "LOC"})
	if _, err := localsync.SyncAll(context.Background(), client, "LOC", dbPath); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// local sql
	var buf bytes.Buffer
	if err := runLocalSQL([]string{"--db", dbPath, "--sql", "SELECT COUNT(*) AS n FROM opportunities"}, &buf, globalOptions{}); err != nil {
		t.Fatalf("local sql: %v", err)
	}
	if !strings.Contains(buf.String(), `"n":`) || !strings.Contains(buf.String(), `2`) {
		t.Fatalf("unexpected sql output: %s", buf.String())
	}

	// local pipeline snapshot
	buf.Reset()
	if err := runLocalCommand([]string{"pipeline", "snapshot", "--db", dbPath}, &buf, globalOptions{}); err != nil {
		t.Fatalf("local pipeline snapshot: %v", err)
	}
	if !strings.Contains(buf.String(), `"pipeline_snapshot"`) {
		t.Fatalf("snapshot output missing view tag: %s", buf.String())
	}

	// local pipeline stale --days 14
	buf.Reset()
	if err := runLocalCommand([]string{"pipeline", "stale", "--days", "14", "--db", dbPath}, &buf, globalOptions{}); err != nil {
		t.Fatalf("local pipeline stale: %v", err)
	}
	var stale map[string]any
	if err := json.Unmarshal(buf.Bytes(), &stale); err != nil {
		t.Fatalf("decode stale: %v\n%s", err, buf.String())
	}
	rows, _ := stale["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 stale row, got %d (%s)", len(rows), buf.String())
	}
	first, _ := rows[0].(map[string]any)
	if first["id"] != "OPP_STALE" {
		t.Fatalf("expected OPP_STALE first, got %v", first["id"])
	}

	// local status
	buf.Reset()
	if err := runLocalCommand([]string{"status", "--db", dbPath}, &buf, globalOptions{}); err != nil {
		t.Fatalf("local status: %v", err)
	}
	if !strings.Contains(buf.String(), `"last_sync_at"`) {
		t.Fatalf("status missing last_sync_at: %s", buf.String())
	}

	// Sanity: schema is real SQLite, not just JSON dumps.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM opportunities WHERE pipeline_id='PIPE1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}
