package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Topline-com/os-cli/internal/topline"
)

// TestSync_NumericCursorPagination is a regression guard for the production bug
// where GHL returned meta.startAfter as a JSON number (epoch ms) and the syncer
// silently dropped it (stringField only matched string), causing every "page 2"
// request to replay the page-1 cursor and the loop to break after one upsert.
//
// The fake here returns startAfter as a number, exactly like GHL prod, and
// asserts that the syncer keeps paging until the server stops handing back a
// cursor — pulling all rows down into SQLite. If this test ever fails again
// with "got 1 opp, expected 3", the regression is back.
func TestSync_NumericCursorPagination(t *testing.T) {
	// Each entity returns three pages; cursor is a numeric epoch-ms value.
	type oppPage struct {
		ID       string
		Cursor   float64
		HasNext  bool
		NextID   string
		NextCur  float64
	}
	oppPages := []oppPage{
		{ID: "O1", HasNext: true, NextID: "O1", NextCur: 1700000001000},
		{ID: "O2", HasNext: true, NextID: "O2", NextCur: 1700000002000},
		{ID: "O3", HasNext: false},
	}
	contactPages := []oppPage{
		{ID: "C1", HasNext: true, NextID: "C1", NextCur: 1700000001000},
		{ID: "C2", HasNext: true, NextID: "C2", NextCur: 1700000002000},
		{ID: "C3", HasNext: false},
	}
	convoPages := []oppPage{
		{ID: "V1", HasNext: true, NextID: "V1", NextCur: 1700000001000},
		{ID: "V2", HasNext: true, NextID: "V2", NextCur: 1700000002000},
		{ID: "V3", HasNext: false},
	}

	var mu sync.Mutex
	oppIdx, contactIdx, convoIdx := 0, 0, 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/opportunities/pipelines":
			_ = json.NewEncoder(w).Encode(map[string]any{"pipelines": []any{}})
		case "/opportunities/search":
			if oppIdx >= len(oppPages) {
				_ = json.NewEncoder(w).Encode(map[string]any{"opportunities": []any{}})
				return
			}
			p := oppPages[oppIdx]
			oppIdx++
			body := map[string]any{
				"opportunities": []any{map[string]any{
					"id":              p.ID,
					"name":            p.ID,
					"status":          "open",
					"pipelineId":      "PIPE1",
					"pipelineStageId": "STG1",
					"contactId":       "X",
					"monetaryValue":   100.0,
				}},
			}
			if p.HasNext {
				body["meta"] = map[string]any{
					"startAfterId": p.NextID,
					"startAfter":   p.NextCur, // numeric — the actual prod shape
				}
			}
			_ = json.NewEncoder(w).Encode(body)
		case "/contacts/search":
			if contactIdx >= len(contactPages) {
				_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []any{}})
				return
			}
			p := contactPages[contactIdx]
			contactIdx++
			body := map[string]any{
				"contacts": []any{map[string]any{
					"id":        p.ID,
					"firstName": p.ID,
					"email":     p.ID + "@example.com",
				}},
			}
			if p.HasNext {
				body["meta"] = map[string]any{
					"startAfterId": p.NextID,
					"startAfter":   p.NextCur,
				}
			}
			_ = json.NewEncoder(w).Encode(body)
		case "/conversations/search":
			if convoIdx >= len(convoPages) {
				_ = json.NewEncoder(w).Encode(map[string]any{"conversations": []any{}})
				return
			}
			p := convoPages[convoIdx]
			convoIdx++
			body := map[string]any{
				"conversations": []any{map[string]any{
					"id":              p.ID,
					"contactId":       "X",
					"lastMessageDate": "2026-05-13T00:00:00Z",
				}},
			}
			if p.HasNext {
				body["meta"] = map[string]any{
					"startAfterId": p.NextID,
					"startAfter":   p.NextCur,
				}
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": map[string]any{"messages": []any{}}})
		}
	}))
	defer ts.Close()

	client := topline.NewClient(topline.Config{BaseURL: ts.URL, PIT: "test", LocationID: "LOC"})
	dbPath := filepath.Join(t.TempDir(), "t.db")
	if _, err := SyncAll(context.Background(), client, "LOC", dbPath); err != nil {
		t.Fatalf("SyncAll: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	for _, c := range []struct {
		table string
		want  int
	}{
		{"opportunities", 3},
		{"contacts", 3},
		{"conversations", 3},
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + c.table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.table, err)
		}
		if n != c.want {
			t.Fatalf("%s: expected %d rows (pagination must follow numeric cursor), got %d", c.table, c.want, n)
		}
	}
}

func TestAnyToString_HandlesNumericCursor(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{1700000001000.0, "1700000001000"},
		{"abc", "abc"},
		{nil, ""},
		{int(42), "42"},
		{int64(1700000001000), "1700000001000"},
		{true, "true"},
	}
	for _, c := range cases {
		got := anyToString(c.in)
		if got != c.want {
			t.Fatalf("anyToString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
