package commands

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
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
	pageLimit := parsePositiveInt(flags["limit"], 100)
	if pageLimit > 100 {
		pageLimit = 100
	}
	opps, err := fetchPipelineOpportunities(context.Background(), client, cfg.LocationID, pipelineID, status, pageLimit, stageNames)
	if err != nil {
		return err
	}
	messages := []reports.MessageEvent(nil)
	tasks := []reports.Task(nil)
	lookupErrors := []reports.LookupError(nil)
	var activityStats *reports.ActivityJoinStats
	activityJoinIncluded := includePipelineActivity(flags)
	if activityJoinIncluded {
		messages, tasks, lookupErrors, activityStats = collectPipelineAuditActivity(context.Background(), client, cfg.LocationID, start, end, opps, flags)
	}
	audit := reports.BuildPipelineAudit(reports.PipelineAuditInput{PipelineName: pipelineName, Start: start, End: end, Opportunities: opps, Messages: messages, Tasks: tasks, ActivityJoinIncluded: activityJoinIncluded, ActivityJoinStats: activityStats})
	audit.LookupErrors = lookupErrors
	return output.WriteJSON(stdout, audit, globals.MaskPII)
}

func parseAuditTime(value string, fallback time.Time) (time.Time, error) {
	return parseAuditTimeWithNow(value, fallback, time.Now())
}

func parseAuditTimeWithNow(value string, fallback time.Time, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "now":
		return now, nil
	case "this-week", "this-week-et", "week", "week-et":
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			return time.Time{}, err
		}
		n := now.In(loc)
		daysSinceMonday := (int(n.Weekday()) + 6) % 7
		weekStart := time.Date(n.Year(), n.Month(), n.Day()-daysSinceMonday, 0, 0, 0, 0, loc)
		return weekStart, nil
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", trimmed); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q; use YYYY-MM-DD, RFC3339, now, or this-week-et", value)
}

func includePipelineActivity(flags map[string]string) bool {
	return !flagIsFalse(flags["includeActivity"]) && !flagIsFalse(flags["activity"]) && flags["skipActivity"] != "true"
}

func flagIsFalse(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.EqualFold(trimmed, "false") || trimmed == "0"
}

func parsePositiveInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

type conversationLookupResult struct {
	ContactID     string
	Conversations []map[string]any
	LookupError   *reports.LookupError
}

type messageLookupResult struct {
	ContactID      string
	ConversationID string
	Messages       []map[string]any
	LookupError    *reports.LookupError
}

type taskLookupResult struct {
	ContactID   string
	Tasks       []map[string]any
	LookupError *reports.LookupError
}

type recentConversationResult struct {
	Conversations []map[string]any
	Complete      bool
	Scanned       int
	LookupError   *reports.LookupError
}

func fetchPipelineOpportunities(ctx context.Context, client *topline.Client, locationID, pipelineID, status string, pageLimit int, stageNames map[string]string) ([]reports.Opportunity, error) {
	query := map[string]string{"location_id": locationID, "pipeline_id": pipelineID, "status": status, "limit": fmt.Sprint(pageLimit)}
	opps := make([]reports.Opportunity, 0)
	seenIDs := map[string]bool{}
	for page := 0; page < 1000; page++ {
		var resp any
		if err := client.Do(ctx, topline.Request{Method: "GET", Path: "/opportunities/search", Query: query}, &resp); err != nil {
			return nil, err
		}
		for _, item := range extractList(resp, "opportunities") {
			opp := opportunityFromMap(item, stageNames)
			if opp.ID != "" {
				if seenIDs[opp.ID] {
					continue
				}
				seenIDs[opp.ID] = true
			}
			opps = append(opps, opp)
		}
		meta, ok := responseMeta(resp)
		if !ok || !hasNextPage(meta) {
			return opps, nil
		}
		startAfter := cursorString(firstAny(meta, "startAfter", "start_after"))
		startAfterID := firstString(meta, "startAfterId", "start_after_id")
		if startAfter == "" || startAfterID == "" {
			return opps, nil
		}
		query["startAfter"] = startAfter
		query["startAfterId"] = startAfterID
	}
	return nil, fmt.Errorf("opportunities pagination exceeded 1000 pages for pipeline %s", pipelineID)
}

func opportunityFromMap(item map[string]any, stageNames map[string]string) reports.Opportunity {
	stageID := firstString(item, "pipelineStageId", "pipeline_stage_id", "stageId")
	return reports.Opportunity{
		ID:            firstString(item, "id", "opportunityId"),
		ContactID:     firstString(item, "contactId", "contact_id"),
		Name:          firstString(item, "name", "title"),
		StageID:       stageID,
		StageName:     firstNonEmpty(stageNames[stageID], firstString(item, "stageName", "pipelineStageName")),
		Status:        firstNonEmpty(firstString(item, "status"), "open"),
		MonetaryValue: numAny(firstAny(item, "monetaryValue", "monetary_value", "value")),
	}
}

func responseMeta(resp any) (map[string]any, bool) {
	m, ok := resp.(map[string]any)
	if !ok {
		return nil, false
	}
	meta, ok := m["meta"].(map[string]any)
	return meta, ok
}

func hasNextPage(meta map[string]any) bool {
	for _, key := range []string{"nextPage", "nextPageUrl", "next"} {
		value, ok := meta[key]
		if !ok || value == nil {
			continue
		}
		if strings.TrimSpace(strAny(value)) != "" {
			return true
		}
	}
	return false
}

func cursorString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case jsonNumber:
		return strings.TrimSpace(fmt.Sprint(v))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func collectPipelineAuditActivity(ctx context.Context, client *topline.Client, locationID string, start, end time.Time, opps []reports.Opportunity, flags map[string]string) ([]reports.MessageEvent, []reports.Task, []reports.LookupError, *reports.ActivityJoinStats) {
	concurrency := parsePositiveInt(flags["concurrency"], 8)
	if concurrency > 16 {
		concurrency = 16
	}
	conversationLimit := parsePositiveInt(firstNonEmpty(flags["conversationLimit"], flags["conversationLimitPerContact"]), 10)
	recentConversationLimit := parsePositiveInt(flags["recentConversationLimit"], 100)
	if recentConversationLimit > 100 {
		recentConversationLimit = 100
	}
	messageLimit := parsePositiveInt(flags["messageLimit"], 30)
	includeTasks := !flagIsFalse(flags["includeTasks"]) && !flagIsFalse(flags["tasks"]) && flags["skipTasks"] != "true"

	contactIDs := uniqueOpportunityContacts(opps)
	stats := &reports.ActivityJoinStats{OpenContacts: len(contactIDs)}
	contactSet := contactIDSet(contactIDs)
	lookupErrors := []reports.LookupError{}
	conversations := []map[string]any{}
	if !flagIsFalse(flags["recentConversationScan"]) && flags["skipRecentConversationScan"] != "true" {
		recent := fetchRecentPipelineConversations(ctx, client, locationID, contactSet, start, recentConversationLimit)
		stats.ConversationSearches++
		stats.ConversationsScanned += recent.Scanned
		if recent.LookupError == nil && recent.Complete {
			stats.Mode = "recent-scan"
			conversations = recent.Conversations
		} else {
			if recent.LookupError != nil {
				lookupErrors = append(lookupErrors, *recent.LookupError)
			}
			stats.Mode = "per-contact-fallback"
			var contactErrors []reports.LookupError
			conversations, contactErrors = fetchPipelineConversations(ctx, client, locationID, contactIDs, conversationLimit, concurrency)
			lookupErrors = append(lookupErrors, contactErrors...)
			stats.ConversationSearches += len(contactIDs)
			stats.ConversationsScanned += len(conversations)
		}
	} else {
		stats.Mode = "per-contact"
		var contactErrors []reports.LookupError
		conversations, contactErrors = fetchPipelineConversations(ctx, client, locationID, contactIDs, conversationLimit, concurrency)
		lookupErrors = append(lookupErrors, contactErrors...)
		stats.ConversationSearches += len(contactIDs)
		stats.ConversationsScanned += len(conversations)
	}

	activeConversations := make([]map[string]any, 0, len(conversations))
	for _, conv := range conversations {
		lastActivity := conversationLastActivity(conv)
		if lastActivity.IsZero() || !lastActivity.Before(start) {
			activeConversations = append(activeConversations, conv)
		}
	}
	stats.ActiveConversations = len(activeConversations)

	messageMaps, messageErrors := fetchPipelineMessages(ctx, client, activeConversations, messageLimit, concurrency)
	lookupErrors = append(lookupErrors, messageErrors...)
	stats.MessageLookups = len(activeConversations)

	messages := make([]reports.MessageEvent, 0, len(messageMaps))
	activeContacts := map[string]bool{}
	for _, item := range messageMaps {
		createdAt := firstCRMTime(item, "dateAdded", "createdAt", "dateCreated", "dateUpdated")
		if createdAt.IsZero() || createdAt.Before(start) || createdAt.After(end) {
			continue
		}
		contactID := firstString(item, "contactId", "contact_id")
		if contactID == "" {
			continue
		}
		messages = append(messages, reports.MessageEvent{
			ContactID:  contactID,
			Type:       normalizeMessageType(firstString(item, "messageType", "type")),
			Direction:  firstString(item, "direction"),
			CreatedAt:  createdAt,
			IsWorkflow: isWorkflowMessage(item),
		})
		activeContacts[contactID] = true
	}

	tasks := []reports.Task(nil)
	if includeTasks && len(activeContacts) > 0 {
		activeContactIDs := make([]string, 0, len(activeContacts))
		for contactID := range activeContacts {
			activeContactIDs = append(activeContactIDs, contactID)
		}
		stats.TaskLookups = len(activeContactIDs)
		taskMaps, taskErrors := fetchPipelineTasks(ctx, client, activeContactIDs, concurrency)
		lookupErrors = append(lookupErrors, taskErrors...)
		for _, item := range taskMaps {
			contactID := firstString(item, "contactId", "contact_id", "contactID")
			tasks = append(tasks, reports.Task{
				ContactID: contactID,
				Title:     firstNonEmpty(firstString(item, "title"), firstString(item, "body"), firstString(item, "description"), "Untitled task"),
				DueAt:     firstCRMTime(item, "dueDate", "dueAt", "date", "due"),
				Completed: boolAny(firstAny(item, "completed", "isCompleted")),
			})
		}
	}

	return messages, tasks, lookupErrors, stats
}

func uniqueOpportunityContacts(opps []reports.Opportunity) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(opps))
	for _, opp := range opps {
		contactID := strings.TrimSpace(opp.ContactID)
		if contactID == "" || seen[contactID] {
			continue
		}
		seen[contactID] = true
		out = append(out, contactID)
	}
	return out
}

func contactIDSet(contactIDs []string) map[string]bool {
	out := make(map[string]bool, len(contactIDs))
	for _, contactID := range contactIDs {
		trimmed := strings.TrimSpace(contactID)
		if trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}

func fetchRecentPipelineConversations(ctx context.Context, client *topline.Client, locationID string, contactIDs map[string]bool, start time.Time, limit int) recentConversationResult {
	var resp any
	err := client.Do(ctx, topline.Request{Method: "GET", Path: "/conversations/search", Query: map[string]string{"locationId": locationID, "status": "all", "limit": fmt.Sprint(limit)}}, &resp)
	if err != nil {
		return recentConversationResult{LookupError: &reports.LookupError{Kind: "recent_conversations", Error: err.Error()}}
	}
	all := extractList(resp, "conversations")
	matched := make([]map[string]any, 0)
	earliest := time.Time{}
	seenConversations := map[string]bool{}
	for _, conv := range all {
		lastActivity := conversationLastActivity(conv)
		if !lastActivity.IsZero() && (earliest.IsZero() || lastActivity.Before(earliest)) {
			earliest = lastActivity
		}
		contactID := firstString(conv, "contactId", "contact_id")
		if !contactIDs[contactID] {
			continue
		}
		if !lastActivity.IsZero() && lastActivity.Before(start) {
			continue
		}
		conversationID := firstString(conv, "id", "conversationId")
		if conversationID != "" && seenConversations[conversationID] {
			continue
		}
		if conversationID != "" {
			seenConversations[conversationID] = true
		}
		matched = append(matched, conv)
	}
	complete := len(all) < limit || (!earliest.IsZero() && earliest.Before(start))
	return recentConversationResult{Conversations: matched, Complete: complete, Scanned: len(all)}
}

func conversationLastActivity(conv map[string]any) time.Time {
	return firstCRMTime(conv, "lastMessageDate", "lastManualMessageDate", "dateUpdated", "updatedAt")
}

func fetchPipelineConversations(ctx context.Context, client *topline.Client, locationID string, contactIDs []string, limit int, concurrency int) ([]map[string]any, []reports.LookupError) {
	jobs := make(chan string)
	results := make(chan conversationLookupResult)
	workerCount := workerCount(concurrency, len(contactIDs))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for contactID := range jobs {
				var resp any
				err := client.Do(ctx, topline.Request{Method: "GET", Path: "/conversations/search", Query: map[string]string{"locationId": locationID, "contactId": contactID, "status": "all", "limit": fmt.Sprint(limit)}}, &resp)
				if err != nil {
					results <- conversationLookupResult{ContactID: contactID, LookupError: &reports.LookupError{Kind: "conversations", ContactID: contactID, Error: err.Error()}}
					continue
				}
				results <- conversationLookupResult{ContactID: contactID, Conversations: extractList(resp, "conversations")}
			}
		}()
	}
	go func() {
		for _, contactID := range contactIDs {
			jobs <- contactID
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	conversations := []map[string]any{}
	lookupErrors := []reports.LookupError{}
	for result := range results {
		if result.LookupError != nil {
			lookupErrors = append(lookupErrors, *result.LookupError)
			continue
		}
		conversations = append(conversations, result.Conversations...)
	}
	return conversations, lookupErrors
}

func fetchPipelineMessages(ctx context.Context, client *topline.Client, conversations []map[string]any, limit int, concurrency int) ([]map[string]any, []reports.LookupError) {
	jobs := make(chan map[string]any)
	results := make(chan messageLookupResult)
	workerCount := workerCount(concurrency, len(conversations))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for conv := range jobs {
				conversationID := firstString(conv, "id", "conversationId")
				contactID := firstString(conv, "contactId", "contact_id")
				if conversationID == "" {
					results <- messageLookupResult{ContactID: contactID, LookupError: &reports.LookupError{Kind: "messages", ContactID: contactID, Error: "conversation id missing"}}
					continue
				}
				var resp any
				err := client.Do(ctx, topline.Request{Method: "GET", Path: "/conversations/" + conversationID + "/messages", Query: map[string]string{"limit": fmt.Sprint(limit)}}, &resp)
				if err != nil {
					results <- messageLookupResult{ContactID: contactID, ConversationID: conversationID, LookupError: &reports.LookupError{Kind: "messages", ContactID: contactID, ConversationID: conversationID, Error: err.Error()}}
					continue
				}
				messages := extractMessages(resp)
				for _, msg := range messages {
					if firstString(msg, "contactId", "contact_id") == "" && contactID != "" {
						msg["contactId"] = contactID
					}
				}
				results <- messageLookupResult{ContactID: contactID, ConversationID: conversationID, Messages: messages}
			}
		}()
	}
	go func() {
		for _, conv := range conversations {
			jobs <- conv
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	messages := []map[string]any{}
	lookupErrors := []reports.LookupError{}
	for result := range results {
		if result.LookupError != nil {
			lookupErrors = append(lookupErrors, *result.LookupError)
			continue
		}
		messages = append(messages, result.Messages...)
	}
	return messages, lookupErrors
}

func fetchPipelineTasks(ctx context.Context, client *topline.Client, contactIDs []string, concurrency int) ([]map[string]any, []reports.LookupError) {
	jobs := make(chan string)
	results := make(chan taskLookupResult)
	workerCount := workerCount(concurrency, len(contactIDs))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for contactID := range jobs {
				var resp any
				err := client.Do(ctx, topline.Request{Method: "GET", Path: "/contacts/" + contactID + "/tasks"}, &resp)
				if err != nil {
					results <- taskLookupResult{ContactID: contactID, LookupError: &reports.LookupError{Kind: "tasks", ContactID: contactID, Error: err.Error()}}
					continue
				}
				tasks := extractList(resp, "tasks")
				for _, task := range tasks {
					if firstString(task, "contactId", "contact_id") == "" {
						task["contactId"] = contactID
					}
				}
				results <- taskLookupResult{ContactID: contactID, Tasks: tasks}
			}
		}()
	}
	go func() {
		for _, contactID := range contactIDs {
			jobs <- contactID
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	tasks := []map[string]any{}
	lookupErrors := []reports.LookupError{}
	for result := range results {
		if result.LookupError != nil {
			lookupErrors = append(lookupErrors, *result.LookupError)
			continue
		}
		tasks = append(tasks, result.Tasks...)
	}
	return tasks, lookupErrors
}

func workerCount(concurrency, jobs int) int {
	if jobs <= 0 {
		return 1
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > jobs {
		return jobs
	}
	return concurrency
}

func extractMessages(value any) []map[string]any {
	if out := extractList(value, "messages"); len(out) > 0 {
		return out
	}
	if m, ok := value.(map[string]any); ok {
		if nested, ok := m["messages"].(map[string]any); ok {
			return extractList(nested, "messages")
		}
	}
	return nil
}

func normalizeMessageType(value string) string {
	s := strings.ToUpper(strings.TrimSpace(value))
	s = strings.TrimPrefix(s, "TYPE_")
	switch {
	case s == "SMS" || s == "TEXT":
		return "SMS"
	case s == "EMAIL" || s == "MAIL":
		return "Email"
	case strings.Contains(s, "CALL") || s == "PHONE":
		return "Call"
	case s == "":
		return "Other"
	default:
		return s
	}
}

func isWorkflowMessage(item map[string]any) bool {
	source := strings.ToLower(firstString(item, "source"))
	return source == "workflow" || source == "campaign" || strings.Contains(strings.ToUpper(firstString(item, "messageType", "type")), "ACTIVITY_APPOINTMENT")
}

func firstCRMTime(m map[string]any, keys ...string) time.Time {
	for _, key := range keys {
		if t := parseCRMTime(firstAny(m, key)); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func parseCRMTime(v any) time.Time {
	switch t := v.(type) {
	case string:
		text := strings.TrimSpace(t)
		if text == "" {
			return time.Time{}
		}
		layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700", "2006-01-02T15:04:05Z0700", "2006-01-02 15:04:05", "2006-01-02"}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed
			}
		}
	case float64:
		if t > 1000000000000 {
			return time.UnixMilli(int64(t))
		}
		if t > 0 {
			return time.Unix(int64(t), 0)
		}
	case int64:
		if t > 1000000000000 {
			return time.UnixMilli(t)
		}
		if t > 0 {
			return time.Unix(t, 0)
		}
	case int:
		return parseCRMTime(int64(t))
	}
	return time.Time{}
}

func boolAny(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(strings.TrimSpace(b), "true") || strings.TrimSpace(b) == "1"
	default:
		return false
	}
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
