package reports

import (
	"sort"
	"time"
)

type Opportunity struct {
	ID            string  `json:"id"`
	ContactID     string  `json:"contactId"`
	Name          string  `json:"name"`
	StageID       string  `json:"stageId"`
	StageName     string  `json:"stageName"`
	Status        string  `json:"status"`
	MonetaryValue float64 `json:"monetaryValue"`
}

type MessageEvent struct {
	ContactID  string    `json:"contactId"`
	Type       string    `json:"type"`
	Direction  string    `json:"direction"`
	CreatedAt  time.Time `json:"createdAt"`
	IsWorkflow bool      `json:"isWorkflow"`
}

type Task struct {
	ContactID string    `json:"contactId"`
	Title     string    `json:"title"`
	DueAt     time.Time `json:"dueAt"`
	Completed bool      `json:"completed"`
}

type PipelineAuditInput struct {
	PipelineName         string         `json:"pipelineName"`
	Start                time.Time      `json:"start"`
	End                  time.Time      `json:"end"`
	Opportunities        []Opportunity  `json:"opportunities"`
	Messages             []MessageEvent `json:"messages"`
	Tasks                []Task         `json:"tasks"`
	ActivityJoinIncluded bool           `json:"activityJoinIncluded"`
	ActivityJoinStats    *ActivityJoinStats
}

type StageSummary struct {
	StageID string  `json:"stageId"`
	Name    string  `json:"name"`
	Deals   int     `json:"deals"`
	Value   float64 `json:"value"`
}

type HygieneFlag struct {
	ContactID string `json:"contactId"`
	Reason    string `json:"reason"`
	Title     string `json:"title,omitempty"`
}

type LookupError struct {
	Kind           string `json:"kind"`
	ContactID      string `json:"contactId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Error          string `json:"error"`
}

type ActivityJoinStats struct {
	Mode                 string `json:"mode"`
	OpenContacts         int    `json:"openContacts"`
	ConversationSearches int    `json:"conversationSearches"`
	ConversationsScanned int    `json:"conversationsScanned"`
	ActiveConversations  int    `json:"activeConversations"`
	MessageLookups       int    `json:"messageLookups"`
	TaskLookups          int    `json:"taskLookups"`
}

type ActiveDealSummary struct {
	OpportunityID         string         `json:"opportunityId"`
	ContactID             string         `json:"contactId"`
	Name                  string         `json:"name"`
	StageName             string         `json:"stageName"`
	MonetaryValue         float64        `json:"monetaryValue"`
	MessageCount          int            `json:"messageCount"`
	HumanActivityCount    int            `json:"humanActivityCount"`
	WorkflowActivityCount int            `json:"workflowActivityCount"`
	ActivityCounts        map[string]int `json:"activityCounts"`
}

type PipelineAudit struct {
	PipelineName          string              `json:"pipelineName"`
	WindowStart           time.Time           `json:"windowStart"`
	WindowEnd             time.Time           `json:"windowEnd"`
	OpenDeals             int                 `json:"openDeals"`
	OpenValue             float64             `json:"openValue"`
	ActivityJoinIncluded  bool                `json:"activityJoinIncluded"`
	ActiveDealsThisWindow int                 `json:"activeDealsThisWindow"`
	ActivityCounts        map[string]int      `json:"activityCounts"`
	StageBreakdown        []StageSummary      `json:"stageBreakdown"`
	HygieneFlags          []HygieneFlag       `json:"hygieneFlags"`
	ActiveOpportunityIDs  []string            `json:"activeOpportunityIds"`
	ActiveDeals           []ActiveDealSummary `json:"activeDeals"`
	ActivityJoinStats     *ActivityJoinStats  `json:"activityJoinStats,omitempty"`
	LookupErrors          []LookupError       `json:"lookupErrors,omitempty"`
	OpenContacts          map[string]string   `json:"-"`
}

func BuildPipelineAudit(in PipelineAuditInput) PipelineAudit {
	a := PipelineAudit{
		PipelineName:         in.PipelineName,
		WindowStart:          in.Start,
		WindowEnd:            in.End,
		ActivityJoinIncluded: in.ActivityJoinIncluded,
		ActivityJoinStats:    in.ActivityJoinStats,
		ActivityCounts:       map[string]int{"Email": 0, "SMS": 0, "Call": 0},
		StageBreakdown:       []StageSummary{},
		HygieneFlags:         []HygieneFlag{},
		ActiveOpportunityIDs: []string{},
		ActiveDeals:          []ActiveDealSummary{},
		OpenContacts:         map[string]string{},
	}
	stages := map[string]*StageSummary{}
	oppByContact := map[string]string{}
	oppDetailsByContact := map[string]Opportunity{}
	for _, opp := range in.Opportunities {
		if opp.Status != "" && opp.Status != "open" {
			continue
		}
		a.OpenDeals++
		a.OpenValue += opp.MonetaryValue
		a.OpenContacts[opp.ContactID] = opp.ID
		oppByContact[opp.ContactID] = opp.ID
		oppDetailsByContact[opp.ContactID] = opp
		key := opp.StageID
		if key == "" {
			key = opp.StageName
		}
		if stages[key] == nil {
			stages[key] = &StageSummary{StageID: opp.StageID, Name: opp.StageName}
		}
		stages[key].Deals++
		stages[key].Value += opp.MonetaryValue
	}
	active := map[string]bool{}
	activeDeals := map[string]*ActiveDealSummary{}
	for _, msg := range in.Messages {
		if msg.CreatedAt.Before(in.Start) || msg.CreatedAt.After(in.End) {
			continue
		}
		oppID := oppByContact[msg.ContactID]
		if oppID == "" {
			continue
		}
		typ := normalizeActivityType(msg.Type)
		a.ActivityCounts[typ]++
		active[oppID] = true
		deal := activeDeals[oppID]
		if deal == nil {
			opp := oppDetailsByContact[msg.ContactID]
			deal = &ActiveDealSummary{OpportunityID: opp.ID, ContactID: opp.ContactID, Name: opp.Name, StageName: opp.StageName, MonetaryValue: opp.MonetaryValue, ActivityCounts: map[string]int{}}
			activeDeals[oppID] = deal
		}
		deal.MessageCount++
		deal.ActivityCounts[typ]++
		if msg.IsWorkflow {
			deal.WorkflowActivityCount++
		} else {
			deal.HumanActivityCount++
		}
	}
	for _, task := range in.Tasks {
		if task.Completed || task.DueAt.IsZero() || !task.DueAt.Before(in.Start) {
			continue
		}
		if _, ok := oppByContact[task.ContactID]; ok {
			a.HygieneFlags = append(a.HygieneFlags, HygieneFlag{ContactID: task.ContactID, Reason: "overdue open task", Title: task.Title})
		}
	}
	for _, s := range stages {
		a.StageBreakdown = append(a.StageBreakdown, *s)
	}
	for id := range active {
		a.ActiveOpportunityIDs = append(a.ActiveOpportunityIDs, id)
	}
	sort.Strings(a.ActiveOpportunityIDs)
	for _, id := range a.ActiveOpportunityIDs {
		if deal := activeDeals[id]; deal != nil {
			a.ActiveDeals = append(a.ActiveDeals, *deal)
		}
	}
	a.ActiveDealsThisWindow = len(a.ActiveOpportunityIDs)
	return a
}

func normalizeActivityType(t string) string {
	switch t {
	case "Email", "email", "Mail", "mail":
		return "Email"
	case "SMS", "sms", "Text", "text":
		return "SMS"
	case "Call", "call", "Phone", "phone":
		return "Call"
	default:
		if t == "" {
			return "Other"
		}
		return t
	}
}
