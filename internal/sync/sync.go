package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Topline-com/os-cli/internal/topline"
)

// Result is the per-entity outcome of a sync run.
type Result struct {
	Entity   string `json:"entity"`
	Fetched  int    `json:"fetched"`
	Upserted int    `json:"upserted"`
	Pages    int    `json:"pages"`
}

// RunResult is the full outcome of a sync run across entities.
type RunResult struct {
	DB        string    `json:"db"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Results   []Result  `json:"results"`
}

// SyncAll runs all available entity syncs against the given SQLite db path.
// Order: pipelines, opportunities, contacts, conversations, messages.
// Contacts after opportunities because we use opportunity.contact_id to scope
// which contacts matter; conversations after contacts; messages after
// conversations.
func SyncAll(ctx context.Context, client *topline.Client, locationID, dbPath string) (RunResult, error) {
	out := RunResult{DB: dbPath, StartedAt: time.Now().UTC()}
	if err := InitDB(dbPath); err != nil {
		return out, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return out, err
	}
	defer db.Close()

	pipelines, err := syncPipelines(ctx, client, db, locationID)
	if err != nil {
		return out, fmt.Errorf("sync pipelines: %w", err)
	}
	out.Results = append(out.Results, pipelines)

	opps, err := syncOpportunities(ctx, client, db, locationID)
	if err != nil {
		return out, fmt.Errorf("sync opportunities: %w", err)
	}
	out.Results = append(out.Results, opps)

	contacts, err := syncContacts(ctx, client, db, locationID)
	if err != nil {
		return out, fmt.Errorf("sync contacts: %w", err)
	}
	out.Results = append(out.Results, contacts)

	convos, err := syncConversations(ctx, client, db, locationID)
	if err != nil {
		return out, fmt.Errorf("sync conversations: %w", err)
	}
	out.Results = append(out.Results, convos)

	msgs, err := syncMessages(ctx, client, db)
	if err != nil {
		return out, fmt.Errorf("sync messages: %w", err)
	}
	out.Results = append(out.Results, msgs)

	if err := setSyncState(db, "last_sync_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return out, err
	}
	out.EndedAt = time.Now().UTC()
	return out, nil
}

func syncPipelines(ctx context.Context, client *topline.Client, db *sql.DB, locationID string) (Result, error) {
	r := Result{Entity: "pipelines", Pages: 1}
	var raw map[string]any
	if err := client.Do(ctx, topline.Request{
		Method: "GET",
		Path:   "/opportunities/pipelines",
		Query:  map[string]string{"locationId": locationID},
	}, &raw); err != nil {
		return r, err
	}
	pipes, _ := raw["pipelines"].([]any)
	tx, err := db.Begin()
	if err != nil {
		return r, err
	}
	pipeStmt, err := tx.Prepare(`INSERT INTO pipelines(id,name,raw_json) VALUES(?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, raw_json=excluded.raw_json`)
	if err != nil {
		_ = tx.Rollback()
		return r, err
	}
	defer pipeStmt.Close()
	stageStmt, err := tx.Prepare(`INSERT INTO pipeline_stages(id,pipeline_id,name,raw_json) VALUES(?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET pipeline_id=excluded.pipeline_id, name=excluded.name, raw_json=excluded.raw_json`)
	if err != nil {
		_ = tx.Rollback()
		return r, err
	}
	defer stageStmt.Close()
	for _, p := range pipes {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		jb, _ := json.Marshal(m)
		if _, err := pipeStmt.Exec(id, name, string(jb)); err != nil {
			_ = tx.Rollback()
			return r, err
		}
		r.Fetched++
		r.Upserted++
		stages, _ := m["stages"].([]any)
		for _, s := range stages {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			sid, _ := sm["id"].(string)
			sname, _ := sm["name"].(string)
			sjb, _ := json.Marshal(sm)
			if _, err := stageStmt.Exec(sid, id, sname, string(sjb)); err != nil {
				_ = tx.Rollback()
				return r, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return r, err
	}
	return r, nil
}

func syncOpportunities(ctx context.Context, client *topline.Client, db *sql.DB, locationID string) (Result, error) {
	r := Result{Entity: "opportunities"}
	startAfterID := ""
	startAfter := ""
	for {
		r.Pages++
		q := map[string]string{
			"location_id": locationID,
			"limit":       "100",
		}
		if startAfterID != "" {
			q["startAfterId"] = startAfterID
		}
		if startAfter != "" {
			q["startAfter"] = startAfter
		}
		var raw map[string]any
		if err := client.Do(ctx, topline.Request{
			Method: "GET",
			Path:   "/opportunities/search",
			Query:  q,
		}, &raw); err != nil {
			return r, err
		}
		list, _ := raw["opportunities"].([]any)
		if len(list) == 0 {
			break
		}
		tx, err := db.Begin()
		if err != nil {
			return r, err
		}
		stmt, err := tx.Prepare(`INSERT INTO opportunities(id,contact_id,pipeline_id,pipeline_stage_id,name,status,monetary_value,raw_json,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				contact_id=excluded.contact_id,
				pipeline_id=excluded.pipeline_id,
				pipeline_stage_id=excluded.pipeline_stage_id,
				name=excluded.name,
				status=excluded.status,
				monetary_value=excluded.monetary_value,
				raw_json=excluded.raw_json,
				updated_at=excluded.updated_at`)
		if err != nil {
			_ = tx.Rollback()
			return r, err
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			name, _ := m["name"].(string)
			status, _ := m["status"].(string)
			pipelineID := stringField(m, "pipelineId", "pipeline_id")
			stageID := stringField(m, "pipelineStageId", "pipeline_stage_id")
			contactID := contactIDFrom(m)
			value := floatField(m, "monetaryValue", "monetary_value")
			updated := stringField(m, "updatedAt", "updated_at", "dateUpdated")
			jb, _ := json.Marshal(m)
			if _, err := stmt.Exec(id, contactID, pipelineID, stageID, name, status, value, string(jb), updated); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return r, err
			}
			r.Fetched++
			r.Upserted++
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return r, err
		}
		// Pagination: GHL search endpoints expose startAfter (epoch ms, encoded
		// as a JSON number) + startAfterId via meta. anyToString handles the
		// numeric case — stringField alone returns "" for float64 values and
		// silently drops the cursor, causing infinite same-page replays.
		meta, _ := raw["meta"].(map[string]any)
		nextID := stringField(meta, "startAfterId", "start_after_id")
		nextAfter := anyToString(meta["startAfter"])
		if nextAfter == "" {
			nextAfter = anyToString(meta["start_after"])
		}
		if nextID == "" || (nextID == startAfterID && nextAfter == startAfter) {
			break
		}
		startAfterID = nextID
		startAfter = nextAfter
	}
	return r, nil
}

// syncContacts pages POST /contacts/search; paginates via meta.startAfter/
// startAfterId returned as a 2-element searchAfter array on subsequent
// requests. Stops when the server returns fewer rows than pageLimit, or when
// the cursor stops advancing.
func syncContacts(ctx context.Context, client *topline.Client, db *sql.DB, locationID string) (Result, error) {
	r := Result{Entity: "contacts"}
	pageLimit := 100
	var searchAfter []any
	for {
		r.Pages++
		body := map[string]any{
			"locationId": locationID,
			"pageLimit":  pageLimit,
		}
		if len(searchAfter) > 0 {
			body["searchAfter"] = searchAfter
		}
		var raw map[string]any
		if err := client.Do(ctx, topline.Request{
			Method: "POST",
			Path:   "/contacts/search",
			Body:   body,
		}, &raw); err != nil {
			return r, err
		}
		list, _ := raw["contacts"].([]any)
		if len(list) == 0 {
			break
		}
		tx, err := db.Begin()
		if err != nil {
			return r, err
		}
		stmt, err := tx.Prepare(`INSERT INTO contacts(id,name,email,phone,raw_json,updated_at)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				name=excluded.name,
				email=excluded.email,
				phone=excluded.phone,
				raw_json=excluded.raw_json,
				updated_at=excluded.updated_at`)
		if err != nil {
			_ = tx.Rollback()
			return r, err
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			name := contactName(m)
			email, _ := m["email"].(string)
			phone, _ := m["phone"].(string)
			updated := stringField(m, "updatedAt", "dateUpdated", "updated_at")
			jb, _ := json.Marshal(m)
			if _, err := stmt.Exec(id, name, email, phone, string(jb), updated); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return r, err
			}
			r.Fetched++
			r.Upserted++
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return r, err
		}
		// Cursor: GHL contacts/search returns meta.startAfter (epoch ms) +
		// meta.startAfterId on the next page. Feed those back as a
		// searchAfter array. Trust the cursor — not the page length — as the
		// stop signal; a "short" page can still be followed by a full one when
		// GHL filters server-side after the fetch.
		meta, _ := raw["meta"].(map[string]any)
		nextID := stringField(meta, "startAfterId", "start_after_id")
		nextAfter := meta["startAfter"]
		if nextAfter == nil {
			nextAfter = meta["start_after"]
		}
		if nextID == "" {
			break
		}
		next := []any{nextAfter, nextID}
		if cursorEqual(next, searchAfter) {
			break
		}
		searchAfter = next
	}
	return r, nil
}

// syncConversations pages GET /conversations/search?locationId=&status=all.
// GHL exposes lastMessageId on each conversation; paginate via meta or by
// taking the last conversation id as the next startAfterId.
func syncConversations(ctx context.Context, client *topline.Client, db *sql.DB, locationID string) (Result, error) {
	r := Result{Entity: "conversations"}
	startAfterID := ""
	startAfter := ""
	for {
		r.Pages++
		q := map[string]string{
			"locationId": locationID,
			"status":     "all",
			"limit":      "100",
		}
		if startAfterID != "" {
			q["startAfterId"] = startAfterID
		}
		if startAfter != "" {
			q["startAfter"] = startAfter
		}
		var raw map[string]any
		if err := client.Do(ctx, topline.Request{
			Method: "GET",
			Path:   "/conversations/search",
			Query:  q,
		}, &raw); err != nil {
			return r, err
		}
		list, _ := raw["conversations"].([]any)
		if len(list) == 0 {
			break
		}
		tx, err := db.Begin()
		if err != nil {
			return r, err
		}
		stmt, err := tx.Prepare(`INSERT INTO conversations(id,contact_id,raw_json,updated_at)
			VALUES(?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				contact_id=excluded.contact_id,
				raw_json=excluded.raw_json,
				updated_at=excluded.updated_at`)
		if err != nil {
			_ = tx.Rollback()
			return r, err
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			contactID := stringField(m, "contactId", "contact_id")
			updated := stringField(m, "lastMessageDate", "dateUpdated", "updatedAt", "updated_at")
			jb, _ := json.Marshal(m)
			if _, err := stmt.Exec(id, contactID, string(jb), updated); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return r, err
			}
			r.Fetched++
			r.Upserted++
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return r, err
		}
		meta, _ := raw["meta"].(map[string]any)
		nextID := stringField(meta, "startAfterId", "start_after_id")
		nextAfter := anyToString(meta["startAfter"])
		if nextAfter == "" {
			nextAfter = anyToString(meta["start_after"])
		}
		if nextID == "" || (nextID == startAfterID && nextAfter == startAfter) {
			break
		}
		startAfterID = nextID
		startAfter = nextAfter
	}
	return r, nil
}

// syncMessages iterates every conversation in the local DB and pages
// GET /conversations/{id}/messages. Idempotent by message id; safe to re-run.
// First pass: bounded to MaxConversationsPerSync to avoid runaway pulls.
func syncMessages(ctx context.Context, client *topline.Client, db *sql.DB) (Result, error) {
	r := Result{Entity: "messages"}
	rows, err := db.Query(`SELECT id, contact_id FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return r, err
	}
	type conv struct{ id, contactID string }
	var convos []conv
	for rows.Next() {
		var c conv
		var cid sql.NullString
		if err := rows.Scan(&c.id, &cid); err != nil {
			_ = rows.Close()
			return r, err
		}
		if cid.Valid {
			c.contactID = cid.String
		}
		convos = append(convos, c)
	}
	_ = rows.Close()
	for _, c := range convos {
		r.Pages++
		var raw map[string]any
		if err := client.Do(ctx, topline.Request{
			Method: "GET",
			Path:   "/conversations/" + c.id + "/messages",
			Query:  map[string]string{"limit": "100"},
		}, &raw); err != nil {
			// Don't fail the whole sync on per-conversation lookup errors;
			// agents can re-run sync to pick them up later.
			continue
		}
		// GHL wraps the actual list under "messages.messages".
		msgs := messagesList(raw)
		if len(msgs) == 0 {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return r, err
		}
		stmt, err := tx.Prepare(`INSERT INTO messages(id,conversation_id,contact_id,type,direction,created_at,raw_json)
			VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				conversation_id=excluded.conversation_id,
				contact_id=excluded.contact_id,
				type=excluded.type,
				direction=excluded.direction,
				created_at=excluded.created_at,
				raw_json=excluded.raw_json`)
		if err != nil {
			_ = tx.Rollback()
			return r, err
		}
		for _, item := range msgs {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			mType := stringField(m, "messageType", "type")
			direction := stringField(m, "direction")
			createdAt := stringField(m, "dateAdded", "createdAt", "created_at")
			contactID := stringField(m, "contactId", "contact_id")
			if contactID == "" {
				contactID = c.contactID
			}
			jb, _ := json.Marshal(m)
			if _, err := stmt.Exec(id, c.id, contactID, mType, direction, createdAt, string(jb)); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return r, err
			}
			r.Fetched++
			r.Upserted++
		}
		_ = stmt.Close()
		if err := tx.Commit(); err != nil {
			return r, err
		}
	}
	return r, nil
}

func messagesList(raw map[string]any) []any {
	if nested, ok := raw["messages"].(map[string]any); ok {
		if l, ok := nested["messages"].([]any); ok {
			return l
		}
	}
	if l, ok := raw["messages"].([]any); ok {
		return l
	}
	return nil
}

func contactName(m map[string]any) string {
	if v := stringField(m, "contactName", "fullNameLowerCase", "fullName", "name"); v != "" {
		return v
	}
	first, _ := m["firstName"].(string)
	last, _ := m["lastName"].(string)
	joined := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	return joined
}

func cursorEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if fmt.Sprint(a[i]) != fmt.Sprint(b[i]) {
			return false
		}
	}
	return true
}

func setSyncState(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO sync_state(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// anyToString converts a JSON-decoded value to its string form, handling the
// numeric case that stringField silently drops. GHL pagination cursors arrive
// as numbers (epoch ms) inside meta.startAfter — formatting them as a string
// without scientific notation is required so we can pass them back in the
// query string for the next page.
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return string(x)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

func floatField(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}
	return 0
}

func contactIDFrom(m map[string]any) string {
	if s := stringField(m, "contactId", "contact_id"); s != "" {
		return s
	}
	if c, ok := m["contact"].(map[string]any); ok {
		return stringField(c, "id")
	}
	return ""
}
