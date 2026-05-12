package reports

import "time"

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
	PipelineName  string         `json:"pipelineName"`
	Start         time.Time      `json:"start"`
	End           time.Time      `json:"end"`
	Opportunities []Opportunity  `json:"opportunities"`
	Messages      []MessageEvent `json:"messages"`
	Tasks         []Task         `json:"tasks"`
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

type PipelineAudit struct {
	PipelineName          string            `json:"pipelineName"`
	WindowStart           time.Time         `json:"windowStart"`
	WindowEnd             time.Time         `json:"windowEnd"`
	OpenDeals             int               `json:"openDeals"`
	OpenValue             float64           `json:"openValue"`
	ActiveDealsThisWindow int               `json:"activeDealsThisWindow"`
	ActivityCounts        map[string]int    `json:"activityCounts"`
	StageBreakdown        []StageSummary    `json:"stageBreakdown"`
	HygieneFlags          []HygieneFlag     `json:"hygieneFlags"`
	ActiveOpportunityIDs  []string          `json:"activeOpportunityIds"`
	OpenContacts          map[string]string `json:"-"`
}

func BuildPipelineAudit(in PipelineAuditInput) PipelineAudit {
	a := PipelineAudit{
		PipelineName:   in.PipelineName,
		WindowStart:    in.Start,
		WindowEnd:      in.End,
		ActivityCounts: map[string]int{"Email": 0, "SMS": 0, "Call": 0},
		OpenContacts:   map[string]string{},
	}
	stages := map[string]*StageSummary{}
	oppByContact := map[string]string{}
	for _, opp := range in.Opportunities {
		if opp.Status != "" && opp.Status != "open" {
			continue
		}
		a.OpenDeals++
		a.OpenValue += opp.MonetaryValue
		a.OpenContacts[opp.ContactID] = opp.ID
		oppByContact[opp.ContactID] = opp.ID
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
