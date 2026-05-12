package commands

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Topline-com/os-cli/internal/output"
	"github.com/Topline-com/os-cli/internal/reports"
	"github.com/Topline-com/os-cli/internal/topline"
)

func runPipelineAudit(args []string, stdout io.Writer, globals globalOptions) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
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

	start, err := parseAuditTime(flags["since"], time.Now().AddDate(0, 0, -7))
	if err != nil {
		return err
	}
	end, err := parseAuditTime(flags["until"], time.Now())
	if err != nil {
		return err
	}
	pipelineID := flags["pipelineId"]
	pipelineName := flags["pipeline"]
	stageNames := map[string]string{}

	var pipelinesResp any
	if err := client.Do(context.Background(), topline.Request{Method: "GET", Path: "/opportunities/pipelines", Query: map[string]string{"locationId": cfg.LocationID}}, &pipelinesResp); err != nil {
		return err
	}
	for _, p := range extractList(pipelinesResp, "pipelines") {
		id := strAny(p["id"])
		name := strAny(p["name"])
		if pipelineID == "" && pipelineName != "" && strings.EqualFold(name, pipelineName) {
			pipelineID = id
		}
		if id == pipelineID && pipelineName == "" {
			pipelineName = name
		}
		for _, s := range extractList(p["stages"], "stages") {
			stageNames[strAny(s["id"])] = strAny(s["name"])
		}
	}
	if pipelineID == "" {
		return fmt.Errorf("pipeline audit requires --pipeline-id or --pipeline")
	}
	if pipelineName == "" {
		pipelineName = pipelineID
	}

	status := flags["status"]
	if status == "" {
		status = "open"
	}
	limit := flags["limit"]
	if limit == "" {
		limit = "100"
	}
	query := map[string]string{"location_id": cfg.LocationID, "pipeline_id": pipelineID, "status": status, "limit": limit}
	var oppResp any
	if err := client.Do(context.Background(), topline.Request{Method: "GET", Path: "/opportunities/search", Query: query}, &oppResp); err != nil {
		return err
	}
	opps := make([]reports.Opportunity, 0)
	for _, item := range extractList(oppResp, "opportunities") {
		stageID := firstString(item, "pipelineStageId", "pipeline_stage_id", "stageId")
		opps = append(opps, reports.Opportunity{
			ID:            firstString(item, "id", "opportunityId"),
			ContactID:     firstString(item, "contactId", "contact_id"),
			Name:          firstString(item, "name", "title"),
			StageID:       stageID,
			StageName:     firstNonEmpty(stageNames[stageID], firstString(item, "stageName", "pipelineStageName")),
			Status:        firstNonEmpty(firstString(item, "status"), "open"),
			MonetaryValue: numAny(firstAny(item, "monetaryValue", "monetary_value", "value")),
		})
	}
	audit := reports.BuildPipelineAudit(reports.PipelineAuditInput{PipelineName: pipelineName, Start: start, End: end, Opportunities: opps})
	return output.WriteJSON(stdout, audit, globals.MaskPII)
}

func parseAuditTime(value string, fallback time.Time) (time.Time, error) {
	if value == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q; use YYYY-MM-DD or RFC3339", value)
}

func extractList(value any, preferred string) []map[string]any {
	if value == nil {
		return nil
	}
	if arr, ok := value.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, v := range arr {
			if m, ok := v.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	keys := []string{preferred, "data", "items", "results", "contacts", "conversations", "messages", "tasks", "notes"}
	for _, key := range keys {
		if arr, ok := m[key].([]any); ok {
			out := make([]map[string]any, 0, len(arr))
			for _, v := range arr {
				if child, ok := v.(map[string]any); ok {
					out = append(out, child)
				}
			}
			return out
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string { return strAny(firstAny(m, keys...)) }
func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func numAny(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case jsonNumber:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

type jsonNumber interface{ Float64() (float64, error) }
