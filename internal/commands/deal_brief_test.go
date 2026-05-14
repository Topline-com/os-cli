package commands

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	localsync "github.com/Topline-com/os-cli/internal/sync"
)

func TestLocalDealBrief(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	if err := localsync.InitDB(dbPath); err != nil {
		t.Fatalf("init: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO pipelines(id,name,raw_json) VALUES('P1','Sales','{}')`)
	exec(`INSERT INTO pipeline_stages(id,pipeline_id,name,raw_json) VALUES('STG1','P1','Proposal Sent','{}')`)
	exec(`INSERT INTO contacts(id,name,email,phone,raw_json,updated_at) VALUES('C1','Jane Doe','jane@x.com','+1','{}','2026-05-12T00:00:00Z')`)
	exec(`INSERT INTO opportunities(id,contact_id,pipeline_id,pipeline_stage_id,name,status,monetary_value,raw_json,updated_at)
	     VALUES('OPP1','C1','P1','STG1','Acme renewal','open',5000.0,'{}','2026-05-13T00:00:00Z')`)
	exec(`INSERT INTO conversations(id,contact_id,raw_json,updated_at) VALUES('CONV1','C1','{}','2026-05-13T00:00:00Z')`)
	exec(`INSERT INTO messages(id,conversation_id,contact_id,type,direction,created_at,raw_json)
	      VALUES('M1','CONV1','C1','TYPE_SMS','inbound','2026-05-13T00:00:00Z','{"body":"hi"}')`)
	exec(`INSERT INTO messages(id,conversation_id,contact_id,type,direction,created_at,raw_json)
	      VALUES('M2','CONV1','C1','TYPE_SMS','outbound','2026-05-13T00:05:00Z','{"body":"reply"}')`)
	exec(`INSERT INTO tasks(id,contact_id,title,due_at,completed,raw_json)
	      VALUES('T1','C1','Follow up','2026-05-15T00:00:00Z',0,'{}')`)
	exec(`INSERT INTO notes(id,contact_id,body,created_at,raw_json)
	      VALUES('N1','C1','warm lead','2026-05-10T00:00:00Z','{}')`)

	var buf bytes.Buffer
	if err := runLocalDealBrief([]string{"--db", dbPath, "--opportunity-id", "OPP1"}, &buf, globalOptions{}); err != nil {
		t.Fatalf("deal brief: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if out["view"] != "deal_brief" || out["found"] != true {
		t.Fatalf("bad header: %v", out)
	}
	opp, _ := out["opportunity"].(map[string]any)
	if opp["name"] != "Acme renewal" || opp["stage"] != "Proposal Sent" || opp["pipeline"] != "Sales" {
		t.Fatalf("bad opportunity: %v", opp)
	}
	contact, _ := out["contact"].(map[string]any)
	if contact["name"] != "Jane Doe" || contact["email"] != "jane@x.com" {
		t.Fatalf("bad contact: %v", contact)
	}
	messages, _ := out["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	first, _ := messages[0].(map[string]any)
	if first["id"] != "M2" {
		t.Fatalf("expected M2 first (DESC by created_at), got %v", first["id"])
	}
	tasks, _ := out["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	notes, _ := out["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	counts, _ := out["counts"].(map[string]any)
	if counts["messages"].(float64) != 2 {
		t.Fatalf("bad counts: %v", counts)
	}
}

func TestLocalDealBrief_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	if err := localsync.InitDB(dbPath); err != nil {
		t.Fatalf("init: %v", err)
	}
	var buf bytes.Buffer
	if err := runLocalDealBrief([]string{"--db", dbPath, "--opportunity-id", "DOES_NOT_EXIST"}, &buf, globalOptions{}); err != nil {
		t.Fatalf("brief: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if out["found"] != false {
		t.Fatalf("expected found=false, got %v", out)
	}
}
