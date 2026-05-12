package commands

import "testing"

func TestMCPParitySpecsIncludeExpectedTools(t *testing.T) {
	expected := []string{
		"topline_ping", "topline_setup_check", "topline_request",
		"topline_search_contacts", "topline_get_contact", "topline_create_contact", "topline_update_contact", "topline_delete_contact", "topline_upsert_contact", "topline_add_contact_tags", "topline_remove_contact_tags", "topline_add_contact_to_workflow", "topline_remove_contact_from_workflow",
		"topline_search_conversations", "topline_get_conversation", "topline_get_messages", "topline_send_message", "topline_create_conversation",
		"topline_list_pipelines", "topline_search_opportunities", "topline_get_opportunity", "topline_create_opportunity", "topline_update_opportunity", "topline_delete_opportunity",
		"topline_list_calendars", "topline_get_calendar", "topline_get_calendar_slots", "topline_create_appointment", "topline_update_appointment", "topline_delete_appointment", "topline_update_calendar", "topline_delete_calendar",
		"topline_list_contact_tasks", "topline_create_task", "topline_update_task", "topline_delete_task",
		"topline_list_contact_notes", "topline_create_note", "topline_update_note", "topline_delete_note",
		"topline_list_custom_fields", "topline_get_custom_field", "topline_create_custom_field", "topline_update_custom_field", "topline_delete_custom_field",
		"topline_list_custom_values", "topline_get_custom_value", "topline_create_custom_value", "topline_update_custom_value", "topline_delete_custom_value",
		"topline_list_workflows", "topline_list_tags", "topline_create_tag", "topline_update_tag", "topline_delete_tag",
		"topline_get_location", "topline_list_users", "topline_get_user", "topline_list_forms", "topline_list_form_submissions", "topline_list_surveys", "topline_list_survey_submissions",
	}
	byTool := SpecsByTool()
	for _, name := range expected {
		if _, ok := byTool[name]; !ok {
			t.Fatalf("missing parity spec %s", name)
		}
	}
}

func TestResolveSpecBuildsPathQueryAndBody(t *testing.T) {
	spec := SpecsByTool()["topline_search_opportunities"]
	req, err := spec.Resolve(map[string]string{
		"pipelineId": "pipe1",
		"status":     "open",
		"limit":      "100",
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if req.Path != "/opportunities/search" {
		t.Fatalf("path mismatch: %q", req.Path)
	}
	if req.Query["pipeline_id"] != "pipe1" || req.Query["status"] != "open" || req.Query["limit"] != "100" {
		t.Fatalf("query mismatch: %#v", req.Query)
	}
	if len(req.Body) != 0 {
		t.Fatalf("GET request should not have body: %#v", req.Body)
	}
}
