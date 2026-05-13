package reports

import (
	"testing"
	"time"
)

func TestBuildPipelineAuditSummarizesOpenValueActivityAndHygiene(t *testing.T) {
	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	audit := BuildPipelineAudit(PipelineAuditInput{
		PipelineName:         "Sales - Flex - Qualified",
		Start:                start,
		End:                  end,
		ActivityJoinIncluded: true,
		Opportunities: []Opportunity{
			{ID: "opp1", ContactID: "c1", Name: "Marcy", StageID: "s1", StageName: "Reviewing Agreement", Status: "open", MonetaryValue: 897},
			{ID: "opp2", ContactID: "c2", Name: "Priya", StageID: "s2", StageName: "Active - This Quarter", Status: "open", MonetaryValue: 997},
			{ID: "opp3", ContactID: "c3", Name: "Won", StageID: "s2", StageName: "Active - This Quarter", Status: "won", MonetaryValue: 500},
		},
		Messages: []MessageEvent{
			{ContactID: "c1", Type: "SMS", Direction: "outbound", CreatedAt: start.Add(time.Hour), IsWorkflow: false},
			{ContactID: "c1", Type: "Email", Direction: "outbound", CreatedAt: start.Add(2 * time.Hour), IsWorkflow: false},
			{ContactID: "c2", Type: "Call", Direction: "outbound", CreatedAt: start.Add(-time.Hour), IsWorkflow: false},
		},
		Tasks: []Task{
			{ContactID: "c2", Title: "Manual follow-up", DueAt: start.Add(-24 * time.Hour), Completed: false},
		},
	})
	if audit.OpenDeals != 2 || audit.OpenValue != 1894 {
		t.Fatalf("open summary mismatch: %#v", audit)
	}
	if !audit.ActivityJoinIncluded {
		t.Fatalf("expected activity join marker")
	}
	if audit.ActiveDealsThisWindow != 1 {
		t.Fatalf("active deal count mismatch: %d", audit.ActiveDealsThisWindow)
	}
	if len(audit.ActiveDeals) != 1 || audit.ActiveDeals[0].OpportunityID != "opp1" || audit.ActiveDeals[0].MessageCount != 2 {
		t.Fatalf("active deal summaries mismatch: %#v", audit.ActiveDeals)
	}
	if audit.ActivityCounts["SMS"] != 1 || audit.ActivityCounts["Email"] != 1 || audit.ActivityCounts["Call"] != 0 {
		t.Fatalf("activity counts mismatch: %#v", audit.ActivityCounts)
	}
	if len(audit.HygieneFlags) != 1 || audit.HygieneFlags[0].ContactID != "c2" {
		t.Fatalf("hygiene flags mismatch: %#v", audit.HygieneFlags)
	}
}
