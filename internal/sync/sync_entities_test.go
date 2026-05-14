package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Topline-com/os-cli/internal/topline"
)

func TestSyncAll_ContactsConversationsMessages(t *testing.T) {
	pipelinesBody := map[string]any{"pipelines": []any{}}
	contactsPage := map[string]any{
		"contacts": []any{
			map[string]any{
				"id":        "C1",
				"firstName": "Jane",
				"lastName":  "Doe",
				"email":     "jane@example.com",
				"phone":     "+15555550101",
				"updatedAt": "2026-05-12T00:00:00Z",
			},
		},
	}
	convosPage := map[string]any{
		"conversations": []any{
			map[string]any{
				"id":              "CONV1",
				"contactId":       "C1",
				"lastMessageDate": "2026-05-13T00:00:00Z",
			},
		},
	}
	msgsBody := map[string]any{
		"messages": map[string]any{
			"messages": []any{
				map[string]any{
					"id":          "M1",
					"contactId":   "C1",
					"messageType": "TYPE_SMS",
					"direction":   "inbound",
					"dateAdded":   "2026-05-13T00:00:00Z",
					"body":        "hi",
				},
				map[string]any{
					"id":          "M2",
					"contactId":   "C1",
					"messageType": "TYPE_SMS",
					"direction":   "outbound",
					"dateAdded":   "2026-05-13T00:05:00Z",
					"body":        "reply",
				},
			},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/opportunities/pipelines":
			_ = json.NewEncoder(w).Encode(pipelinesBody)
		case r.URL.Path == "/opportunities/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"opportunities": []any{}})
		case r.URL.Path == "/contacts/search":
			_ = json.NewEncoder(w).Encode(contactsPage)
		case r.URL.Path == "/conversations/search":
			_ = json.NewEncoder(w).Encode(convosPage)
		case strings.HasPrefix(r.URL.Path, "/conversations/") && strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(msgsBody)
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
	entities := map[string]Result{}
	for _, r := range res.Results {
		entities[r.Entity] = r
	}
	if entities["contacts"].Upserted != 1 {
		t.Fatalf("expected 1 contact upserted, got %d", entities["contacts"].Upserted)
	}
	if entities["conversations"].Upserted != 1 {
		t.Fatalf("expected 1 conversation, got %d", entities["conversations"].Upserted)
	}
	if entities["messages"].Upserted != 2 {
		t.Fatalf("expected 2 messages, got %d", entities["messages"].Upserted)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	var name, email string
	if err := db.QueryRow(`SELECT name, email FROM contacts WHERE id='C1'`).Scan(&name, &email); err != nil {
		t.Fatalf("contact: %v", err)
	}
	if name != "Jane Doe" || email != "jane@example.com" {
		t.Fatalf("unexpected contact: name=%q email=%q", name, email)
	}
	var convoCID string
	if err := db.QueryRow(`SELECT contact_id FROM conversations WHERE id='CONV1'`).Scan(&convoCID); err != nil {
		t.Fatalf("conv: %v", err)
	}
	if convoCID != "C1" {
		t.Fatalf("conv contact_id %q", convoCID)
	}
	var inbound, outbound int
	if err := db.QueryRow(`SELECT
	  SUM(CASE WHEN direction='inbound' THEN 1 ELSE 0 END),
	  SUM(CASE WHEN direction='outbound' THEN 1 ELSE 0 END)
	FROM messages WHERE conversation_id='CONV1'`).Scan(&inbound, &outbound); err != nil {
		t.Fatalf("msg counts: %v", err)
	}
	if inbound != 1 || outbound != 1 {
		t.Fatalf("expected 1/1 in/out, got %d/%d", inbound, outbound)
	}

	// Idempotent re-run.
	if _, err := SyncAll(context.Background(), client, "LOC", dbPath); err != nil {
		t.Fatalf("second SyncAll: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n); err != nil {
		t.Fatalf("recount: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected idempotent message count 2, got %d", n)
	}
}
