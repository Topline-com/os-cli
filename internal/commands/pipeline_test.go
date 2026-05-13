package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPipelineAuditFetchesActivityConcurrently(t *testing.T) {
	t.Setenv("TOPLINE_PIT", "example-token")
	t.Setenv("TOPLINE_LOCATION_ID", "loc_default")

	var mu sync.Mutex
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/opportunities/pipelines":
			if got := r.URL.Query().Get("locationId"); got != "loc_123" {
				t.Fatalf("locationId mismatch for pipelines: %q", got)
			}
			_, _ = w.Write([]byte(`{
				"pipelines":[{"id":"pipe_qualified","name":"Qualified","stages":[{"id":"stage_review","name":"Reviewing Agreement"},{"id":"stage_active","name":"Active"}]}]
			}`))
		case "/opportunities/search":
			q := r.URL.Query()
			if q.Get("location_id") != "loc_123" || q.Get("pipeline_id") != "pipe_qualified" || q.Get("status") != "open" || q.Get("limit") != "10" {
				t.Fatalf("opportunity query mismatch: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{
				"opportunities":[
					{"id":"opp_marcy","contactId":"contact_marcy","name":"Marcy Miller","pipelineStageId":"stage_review","status":"open","monetaryValue":897},
					{"id":"opp_scottessa","contactId":"contact_scottessa","name":"Scottessa Hurte","pipelineStageId":"stage_active","status":"open","monetaryValue":997}
				]
			}`))
		case "/conversations/search":
			q := r.URL.Query()
			if q.Get("locationId") != "loc_123" || q.Get("status") != "all" {
				t.Fatalf("conversation query mismatch: %s", r.URL.RawQuery)
			}
			if q.Get("contactId") != "" {
				t.Fatalf("expected recent conversation scan, got per-contact lookup: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"conversations":[
				{"id":"conv_marcy","contactId":"contact_marcy","lastMessageDate":"2026-05-12T18:00:00Z"},
				{"id":"conv_scottessa","contactId":"contact_scottessa","lastMessageDate":"2026-05-01T18:00:00Z"}
			]}`))
		case "/conversations/conv_marcy/messages":
			_, _ = w.Write([]byte(`{
				"messages":{"messages":[
					{"id":"msg_sms","contactId":"contact_marcy","messageType":"TYPE_SMS","direction":"outbound","source":"manual","dateAdded":"2026-05-12T18:00:00Z"},
					{"id":"msg_email","contactId":"contact_marcy","messageType":"TYPE_EMAIL","direction":"outbound","source":"manual","dateAdded":"2026-05-12T19:00:00Z"}
				]}
			}`))
		case "/contacts/contact_marcy/tasks":
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task_1","title":"Send pricing","dueDate":"2026-05-10T12:00:00Z","completed":false}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := Execute([]string{
		"--base-url", server.URL,
		"--location-id", "loc_123",
		"--agent",
		"pipeline", "audit",
		"--pipeline-id", "pipe_qualified",
		"--since", "2026-05-11T00:00:00Z",
		"--until", "2026-05-13T00:00:00Z",
		"--limit", "10",
		"--concurrency", "2",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute returned error: %v\nstderr=%s", err, stderr.String())
	}

	var out struct {
		PipelineName          string         `json:"pipelineName"`
		OpenDeals             int            `json:"openDeals"`
		ActivityJoinIncluded  bool           `json:"activityJoinIncluded"`
		ActiveDealsThisWindow int            `json:"activeDealsThisWindow"`
		ActivityCounts        map[string]int `json:"activityCounts"`
		ActiveOpportunityIDs  []string       `json:"activeOpportunityIds"`
		ActiveDeals           []struct {
			OpportunityID  string         `json:"opportunityId"`
			Name           string         `json:"name"`
			MessageCount   int            `json:"messageCount"`
			ActivityCounts map[string]int `json:"activityCounts"`
		} `json:"activeDeals"`
		ActivityJoinStats struct {
			Mode                 string `json:"mode"`
			ConversationSearches int    `json:"conversationSearches"`
			ConversationsScanned int    `json:"conversationsScanned"`
			ActiveConversations  int    `json:"activeConversations"`
			MessageLookups       int    `json:"messageLookups"`
			TaskLookups          int    `json:"taskLookups"`
		} `json:"activityJoinStats"`
		HygieneFlags []struct {
			ContactID string `json:"contactId"`
			Title     string `json:"title"`
		} `json:"hygieneFlags"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if out.PipelineName != "Qualified" || out.OpenDeals != 2 {
		t.Fatalf("pipeline/open summary mismatch: %#v", out)
	}
	if !out.ActivityJoinIncluded {
		t.Fatalf("expected activityJoinIncluded=true so zero activity reports can be trusted")
	}
	if out.ActiveDealsThisWindow != 1 {
		t.Fatalf("expected one active deal from joined messages, got %d; output=%s", out.ActiveDealsThisWindow, stdout.String())
	}
	if out.ActivityCounts["SMS"] != 1 || out.ActivityCounts["Email"] != 1 {
		t.Fatalf("activity counts were not built from conversation messages: %#v", out.ActivityCounts)
	}
	sort.Strings(out.ActiveOpportunityIDs)
	if strings.Join(out.ActiveOpportunityIDs, ",") != "opp_marcy" {
		t.Fatalf("active opportunity ids mismatch: %#v", out.ActiveOpportunityIDs)
	}
	if len(out.ActiveDeals) != 1 || out.ActiveDeals[0].OpportunityID != "opp_marcy" || out.ActiveDeals[0].Name != "Marcy Miller" || out.ActiveDeals[0].MessageCount != 2 {
		t.Fatalf("expected active deal details to avoid follow-up lookups, got %#v", out.ActiveDeals)
	}
	if out.ActiveDeals[0].ActivityCounts["SMS"] != 1 || out.ActiveDeals[0].ActivityCounts["Email"] != 1 {
		t.Fatalf("active deal activity counts mismatch: %#v", out.ActiveDeals[0].ActivityCounts)
	}
	if len(out.HygieneFlags) != 1 || out.HygieneFlags[0].ContactID != "contact_marcy" || out.HygieneFlags[0].Title != "Send pricing" {
		t.Fatalf("expected overdue task hygiene flag from active contact, got %#v", out.HygieneFlags)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen["/conversations/search"] != 1 {
		t.Fatalf("expected one recent conversation scan instead of N contact lookups, saw %#v", seen)
	}
	if out.ActivityJoinStats.Mode != "recent-scan" || out.ActivityJoinStats.ConversationSearches != 1 || out.ActivityJoinStats.ConversationsScanned != 2 || out.ActivityJoinStats.ActiveConversations != 1 || out.ActivityJoinStats.MessageLookups != 1 || out.ActivityJoinStats.TaskLookups != 1 {
		t.Fatalf("activity join stats did not describe the fast path: %#v", out.ActivityJoinStats)
	}
	if seen["/conversations/conv_marcy/messages"] != 1 {
		t.Fatalf("expected message lookup for active conversation, saw %#v", seen)
	}
	if seen["/contacts/contact_marcy/tasks"] != 1 {
		t.Fatalf("expected task lookup for active deal contact, saw %#v", seen)
	}
}

func TestParseAuditTimeThisWeekET(t *testing.T) {
	now := time.Date(2026, 5, 12, 20, 30, 0, 0, time.UTC)
	got, err := parseAuditTimeWithNow("this-week-et", time.Time{}, now)
	if err != nil {
		t.Fatalf("parseAuditTimeWithNow returned error: %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 5, 11, 0, 0, 0, 0, loc)
	if !got.Equal(want) || got.Location().String() != loc.String() {
		t.Fatalf("this-week-et = %s (%s), want %s (%s)", got, got.Location(), want, want.Location())
	}
}
