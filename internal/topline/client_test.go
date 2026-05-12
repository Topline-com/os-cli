package topline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRequestAddsAuthVersionQueryAndBody(t *testing.T) {
	var seenPath, seenAuth, seenVersion string
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.String()
		seenAuth = r.Header.Get("Authorization")
		seenVersion = r.Header.Get("Version")
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(Config{PIT: "pit-secret", LocationID: "loc_123", BaseURL: server.URL})
	var out map[string]any
	err := client.Do(context.Background(), Request{
		Method: "POST",
		Path:   "/contacts/",
		Query:  map[string]string{"locationId": "loc_123", "limit": "1"},
		Body:   map[string]any{"firstName": "Jane"},
	}, &out)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if seenPath != "/contacts/?limit=1&locationId=loc_123" {
		t.Fatalf("path mismatch: %q", seenPath)
	}
	if seenAuth != "Bearer pit-secret" {
		t.Fatalf("auth mismatch: %q", seenAuth)
	}
	if seenVersion != APIVersion {
		t.Fatalf("version mismatch: %q", seenVersion)
	}
	if seenBody["firstName"] != "Jane" {
		t.Fatalf("body mismatch: %#v", seenBody)
	}
}
