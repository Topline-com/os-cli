package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Topline-com/os-cli/internal/topline"
)

type Spec struct {
	ToolName       string
	Command        []string
	Method         string
	Path           string
	Required       []string
	QueryParams    map[string]string
	BodyParams     map[string]string
	LocationTarget string // query, body, or path
	LocationAPI    string // locationId or location_id
}

func (s Spec) Resolve(args map[string]string) (topline.Request, error) {
	for _, required := range s.Required {
		if strings.TrimSpace(args[required]) == "" {
			return topline.Request{}, fmt.Errorf("missing required --%s", required)
		}
	}
	path := s.Path
	usedPath := map[string]bool{}
	for key, value := range args {
		ph := "{" + key + "}"
		if strings.Contains(path, ph) {
			path = strings.ReplaceAll(path, ph, value)
			usedPath[key] = true
		}
	}
	if strings.Contains(path, "{") {
		return topline.Request{}, fmt.Errorf("unresolved path parameter in %s", path)
	}
	req := topline.Request{Method: s.Method, Path: path, Query: map[string]string{}, Body: map[string]any{}}
	if req.Method == "" {
		req.Method = "GET"
	}
	method := strings.ToUpper(req.Method)
	for key, value := range args {
		if usedPath[key] || value == "" {
			continue
		}
		if apiName, ok := s.QueryParams[key]; ok {
			req.Query[apiName] = value
			continue
		}
		if apiName, ok := s.BodyParams[key]; ok {
			req.Body[apiName] = parseValue(key, value)
			continue
		}
		if method == "GET" || method == "DELETE" {
			req.Query[key] = value
		} else {
			req.Body[key] = parseValue(key, value)
		}
	}
	if len(req.Body) == 0 {
		req.Body = nil
	}
	return req, nil
}

func (s Spec) WithLocation(args map[string]string, locationID string) map[string]string {
	if s.LocationTarget == "" || args["locationId"] != "" || locationID == "" {
		return args
	}
	copyArgs := map[string]string{}
	for k, v := range args {
		copyArgs[k] = v
	}
	copyArgs["locationId"] = locationID
	return copyArgs
}

func parseValue(key, value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var decoded any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return decoded
		}
	}
	if trimmed == "true" {
		return true
	}
	if trimmed == "false" {
		return false
	}
	if (key == "tags" || strings.HasSuffix(key, "Tags")) && strings.Contains(trimmed, ",") {
		parts := strings.Split(trimmed, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return value
}

func SpecsByTool() map[string]Spec {
	out := map[string]Spec{}
	for _, spec := range Specs() {
		out[spec.ToolName] = spec
	}
	return out
}

func SpecsByCommand() map[string]Spec {
	out := map[string]Spec{}
	for _, spec := range Specs() {
		if len(spec.Command) > 0 {
			out[strings.Join(spec.Command, " ")] = spec
		}
		out[spec.ToolName] = spec
	}
	return out
}

func loc(target string) (string, string)        { return target, "locationId" }
func locID(target, api string) (string, string) { return target, api }

func spec(tool string, cmd []string, method, path string, required []string, locationTarget string, locationAPI string, query map[string]string, body map[string]string) Spec {
	return Spec{ToolName: tool, Command: cmd, Method: method, Path: path, Required: required, LocationTarget: locationTarget, LocationAPI: locationAPI, QueryParams: query, BodyParams: body}
}

func Specs() []Spec {
	lq, lqName := loc("query")
	lb, lbName := loc("body")
	lp, lpName := loc("path")
	lqSnake, lqSnakeName := locID("query", "location_id")
	return []Spec{
		spec("topline_ping", []string{"ping"}, "GET", "/locations/{locationId}", nil, lp, lpName, nil, nil),
		spec("topline_setup_check", []string{"setup-check"}, "GET", "/locations/{locationId}", nil, lp, lpName, nil, nil),
		spec("topline_request", []string{"raw", "request"}, "", "", nil, "", "", nil, nil),

		spec("topline_search_contacts", []string{"contacts", "search"}, "POST", "/contacts/search", nil, lb, lbName, nil, map[string]string{"query": "query", "tags": "tags", "limit": "pageLimit", "startAfterId": "searchAfter", "locationId": "locationId"}),
		spec("topline_get_contact", []string{"contacts", "get"}, "GET", "/contacts/{contactId}", []string{"contactId"}, "", "", nil, nil),
		spec("topline_create_contact", []string{"contacts", "create"}, "POST", "/contacts/", nil, lb, lbName, nil, map[string]string{"locationId": "locationId"}),
		spec("topline_update_contact", []string{"contacts", "update"}, "PUT", "/contacts/{contactId}", []string{"contactId"}, "", "", nil, nil),
		spec("topline_delete_contact", []string{"contacts", "delete"}, "DELETE", "/contacts/{contactId}", []string{"contactId"}, "", "", nil, nil),
		spec("topline_add_contact_tags", []string{"contacts", "tags", "add"}, "POST", "/contacts/{contactId}/tags", []string{"contactId", "tags"}, "", "", nil, map[string]string{"tags": "tags"}),
		spec("topline_remove_contact_tags", []string{"contacts", "tags", "remove"}, "DELETE", "/contacts/{contactId}/tags", []string{"contactId", "tags"}, "", "", nil, map[string]string{"tags": "tags"}),
		spec("topline_upsert_contact", []string{"contacts", "upsert"}, "POST", "/contacts/upsert", nil, lb, lbName, nil, map[string]string{"locationId": "locationId"}),
		spec("topline_add_contact_to_workflow", []string{"contacts", "workflow", "add"}, "POST", "/contacts/{contactId}/workflow/{workflowId}", []string{"contactId", "workflowId"}, "", "", nil, nil),
		spec("topline_remove_contact_from_workflow", []string{"contacts", "workflow", "remove"}, "DELETE", "/contacts/{contactId}/workflow/{workflowId}", []string{"contactId", "workflowId"}, "", "", nil, nil),

		spec("topline_search_conversations", []string{"conversations", "search"}, "GET", "/conversations/search", nil, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_get_conversation", []string{"conversations", "get"}, "GET", "/conversations/{conversationId}", []string{"conversationId"}, "", "", nil, nil),
		spec("topline_get_messages", []string{"conversations", "messages"}, "GET", "/conversations/{conversationId}/messages", []string{"conversationId"}, "", "", nil, nil),
		spec("topline_send_message", []string{"conversations", "send"}, "POST", "/conversations/messages", []string{"contactId", "type"}, "", "", nil, nil),
		spec("topline_create_conversation", []string{"conversations", "create"}, "POST", "/conversations/", []string{"contactId"}, lb, lbName, nil, map[string]string{"locationId": "locationId"}),

		spec("topline_list_pipelines", []string{"opportunities", "pipelines"}, "GET", "/opportunities/pipelines", nil, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_search_opportunities", []string{"opportunities", "search"}, "GET", "/opportunities/search", nil, lqSnake, lqSnakeName, map[string]string{"locationId": "location_id", "query": "q", "pipelineId": "pipeline_id", "pipelineStageId": "pipeline_stage_id", "assignedTo": "assigned_to", "contactId": "contact_id", "status": "status", "limit": "limit", "startAfterId": "startAfterId"}, nil),
		spec("topline_get_opportunity", []string{"opportunities", "get"}, "GET", "/opportunities/{opportunityId}", []string{"opportunityId"}, "", "", nil, nil),
		spec("topline_create_opportunity", []string{"opportunities", "create"}, "POST", "/opportunities/", []string{"pipelineId", "pipelineStageId", "contactId", "name"}, lb, lbName, nil, map[string]string{"locationId": "locationId"}),
		spec("topline_update_opportunity", []string{"opportunities", "update"}, "PUT", "/opportunities/{opportunityId}", []string{"opportunityId"}, "", "", nil, nil),
		spec("topline_delete_opportunity", []string{"opportunities", "delete"}, "DELETE", "/opportunities/{opportunityId}", []string{"opportunityId"}, "", "", nil, nil),

		spec("topline_list_calendars", []string{"calendars", "list"}, "GET", "/calendars/", nil, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_get_calendar", []string{"calendars", "get"}, "GET", "/calendars/{calendarId}", []string{"calendarId"}, "", "", nil, nil),
		spec("topline_get_calendar_slots", []string{"calendars", "slots"}, "GET", "/calendars/{calendarId}/free-slots", []string{"calendarId", "startDate", "endDate"}, "", "", nil, nil),
		spec("topline_create_appointment", []string{"calendars", "appointments", "create"}, "POST", "/calendars/events/appointments", []string{"calendarId", "contactId", "startTime"}, lb, lbName, nil, map[string]string{"locationId": "locationId"}),
		spec("topline_update_appointment", []string{"calendars", "appointments", "update"}, "PUT", "/calendars/events/appointments/{appointmentId}", []string{"appointmentId"}, "", "", nil, nil),
		spec("topline_delete_appointment", []string{"calendars", "appointments", "delete"}, "DELETE", "/calendars/events/appointments/{appointmentId}", []string{"appointmentId"}, "", "", nil, nil),
		spec("topline_update_calendar", []string{"calendars", "update"}, "PUT", "/calendars/{calendarId}", []string{"calendarId"}, "", "", nil, nil),
		spec("topline_delete_calendar", []string{"calendars", "delete"}, "DELETE", "/calendars/{calendarId}", []string{"calendarId"}, "", "", nil, nil),

		spec("topline_list_contact_tasks", []string{"tasks", "list"}, "GET", "/contacts/{contactId}/tasks", []string{"contactId"}, "", "", nil, nil),
		spec("topline_create_task", []string{"tasks", "create"}, "POST", "/contacts/{contactId}/tasks", []string{"contactId", "title", "dueDate"}, "", "", nil, nil),
		spec("topline_update_task", []string{"tasks", "update"}, "PUT", "/contacts/{contactId}/tasks/{taskId}", []string{"contactId", "taskId"}, "", "", nil, nil),
		spec("topline_delete_task", []string{"tasks", "delete"}, "DELETE", "/contacts/{contactId}/tasks/{taskId}", []string{"contactId", "taskId"}, "", "", nil, nil),

		spec("topline_list_contact_notes", []string{"notes", "list"}, "GET", "/contacts/{contactId}/notes", []string{"contactId"}, "", "", nil, nil),
		spec("topline_create_note", []string{"notes", "create"}, "POST", "/contacts/{contactId}/notes", []string{"contactId", "body"}, "", "", nil, nil),
		spec("topline_update_note", []string{"notes", "update"}, "PUT", "/contacts/{contactId}/notes/{noteId}", []string{"contactId", "noteId", "body"}, "", "", nil, nil),
		spec("topline_delete_note", []string{"notes", "delete"}, "DELETE", "/contacts/{contactId}/notes/{noteId}", []string{"contactId", "noteId"}, "", "", nil, nil),

		spec("topline_list_custom_fields", []string{"custom-fields", "list"}, "GET", "/locations/{locationId}/customFields", nil, lp, lpName, nil, nil),
		spec("topline_get_custom_field", []string{"custom-fields", "get"}, "GET", "/locations/{locationId}/customFields/{customFieldId}", []string{"customFieldId"}, lp, lpName, nil, nil),
		spec("topline_create_custom_field", []string{"custom-fields", "create"}, "POST", "/locations/{locationId}/customFields", []string{"name", "dataType"}, lp, lpName, nil, map[string]string{"locationId": "-"}),
		spec("topline_update_custom_field", []string{"custom-fields", "update"}, "PUT", "/locations/{locationId}/customFields/{customFieldId}", []string{"customFieldId"}, lp, lpName, nil, nil),
		spec("topline_delete_custom_field", []string{"custom-fields", "delete"}, "DELETE", "/locations/{locationId}/customFields/{customFieldId}", []string{"customFieldId"}, lp, lpName, nil, nil),

		spec("topline_list_custom_values", []string{"custom-values", "list"}, "GET", "/locations/{locationId}/customValues", nil, lp, lpName, nil, nil),
		spec("topline_get_custom_value", []string{"custom-values", "get"}, "GET", "/locations/{locationId}/customValues/{customValueId}", []string{"customValueId"}, lp, lpName, nil, nil),
		spec("topline_create_custom_value", []string{"custom-values", "create"}, "POST", "/locations/{locationId}/customValues", []string{"name", "value"}, lp, lpName, nil, nil),
		spec("topline_update_custom_value", []string{"custom-values", "update"}, "PUT", "/locations/{locationId}/customValues/{customValueId}", []string{"customValueId"}, lp, lpName, nil, nil),
		spec("topline_delete_custom_value", []string{"custom-values", "delete"}, "DELETE", "/locations/{locationId}/customValues/{customValueId}", []string{"customValueId"}, lp, lpName, nil, nil),

		spec("topline_list_workflows", []string{"workflows", "list"}, "GET", "/workflows/", nil, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_list_tags", []string{"tags", "list"}, "GET", "/locations/{locationId}/tags", nil, lp, lpName, nil, nil),
		spec("topline_create_tag", []string{"tags", "create"}, "POST", "/locations/{locationId}/tags", []string{"name"}, lp, lpName, nil, nil),
		spec("topline_update_tag", []string{"tags", "update"}, "PUT", "/locations/{locationId}/tags/{tagId}", []string{"tagId", "name"}, lp, lpName, nil, nil),
		spec("topline_delete_tag", []string{"tags", "delete"}, "DELETE", "/locations/{locationId}/tags/{tagId}", []string{"tagId"}, lp, lpName, nil, nil),

		spec("topline_get_location", []string{"location", "get"}, "GET", "/locations/{locationId}", nil, lp, lpName, nil, nil),
		spec("topline_list_users", []string{"users", "list"}, "GET", "/users/", nil, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_get_user", []string{"users", "get"}, "GET", "/users/{userId}", []string{"userId"}, "", "", nil, nil),
		spec("topline_list_forms", []string{"forms", "list"}, "GET", "/forms/", nil, lq, lqName, map[string]string{"locationId": "locationId", "startAfterId": "skip"}, nil),
		spec("topline_list_form_submissions", []string{"forms", "submissions"}, "GET", "/forms/submissions", []string{"formId"}, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
		spec("topline_list_surveys", []string{"surveys", "list"}, "GET", "/surveys/", nil, lq, lqName, map[string]string{"locationId": "locationId", "startAfterId": "skip"}, nil),
		spec("topline_list_survey_submissions", []string{"surveys", "submissions"}, "GET", "/surveys/submissions", []string{"surveyId"}, lq, lqName, map[string]string{"locationId": "locationId"}, nil),
	}
}
