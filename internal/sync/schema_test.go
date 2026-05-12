package sync

import "testing"

func TestSchemaIncludesCoreMirrorTables(t *testing.T) {
	for _, table := range []string{"contacts", "opportunities", "pipelines", "pipeline_stages", "conversations", "messages", "tasks", "notes", "sync_state"} {
		if !SchemaContainsTable(table) {
			t.Fatalf("schema missing table %s", table)
		}
	}
}
