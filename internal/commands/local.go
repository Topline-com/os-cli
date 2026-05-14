package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Topline-com/os-cli/internal/output"
	localsync "github.com/Topline-com/os-cli/internal/sync"
	"github.com/Topline-com/os-cli/internal/topline"
)

// runLocalCommand dispatches `topline local …` — the native, offline-first
// command family that reads/writes the local SQLite mirror.
func runLocalCommand(args []string, stdout io.Writer, globals globalOptions) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printLocalHelp(stdout)
		return nil
	}
	switch args[0] {
	case "sync":
		return runLocalSync(args[1:], stdout, globals)
	case "sql":
		return runLocalSQL(args[1:], stdout, globals)
	case "pipeline":
		return runLocalPipeline(args[1:], stdout, globals)
	case "status":
		return runLocalStatus(args[1:], stdout, globals)
	case "deal":
		return runLocalDeal(args[1:], stdout, globals)
	default:
		return fmt.Errorf("unknown local subcommand %q; try `topline local help`", args[0])
	}
}

func printLocalHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Topline OS - native commands (local SQLite mirror)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  topline local sync                       # full sync into ~/.topline/state.db")
	_, _ = fmt.Fprintln(w, "  topline local status                     # last sync, row counts, freshness")
	_, _ = fmt.Fprintln(w, "  topline local sql --sql 'SELECT ...'     # query the local mirror")
	_, _ = fmt.Fprintln(w, "  topline local pipeline snapshot --pipeline-id PIPE")
	_, _ = fmt.Fprintln(w, "  topline local pipeline stale --days 14 [--pipeline-id PIPE]")
	_, _ = fmt.Fprintln(w, "  topline local deal brief --opportunity-id OPP [--messages N]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --db PATH    override DB path (default: $TOPLINE_DB or ~/.topline/state.db)")
}

func defaultDBPath() (string, error) {
	if env := strings.TrimSpace(os.Getenv("TOPLINE_DB")); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".topline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.db"), nil
}

func resolveDBPath(flags map[string]string) (string, error) {
	if p := flags["db"]; p != "" {
		return p, nil
	}
	return defaultDBPath()
}

func runLocalSync(args []string, stdout io.Writer, globals globalOptions) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	dbPath, err := resolveDBPath(flags)
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
	res, err := localsync.SyncAll(context.Background(), client, cfg.LocationID, dbPath)
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, res, globals.MaskPII)
}

func runLocalSQL(args []string, stdout io.Writer, globals globalOptions) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	sqlText := strings.TrimSpace(flags["sql"])
	if sqlText == "" && flags["file"] != "" {
		b, readErr := os.ReadFile(flags["file"])
		if readErr != nil {
			return readErr
		}
		sqlText = strings.TrimSpace(string(b))
	}
	if sqlText == "" {
		return errors.New("usage: topline local sql --sql 'SELECT ...' [--db PATH]")
	}
	dbPath, err := resolveDBPath(flags)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, columns, err := executeLocalSQL(context.Background(), db, sqlText)
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, map[string]any{
		"columns": columns,
		"rows":    rows,
		"count":   len(rows),
	}, globals.MaskPII)
}

func executeLocalSQL(ctx context.Context, db *sql.DB, sqlText string) ([]map[string]any, []string, error) {
	rs, err := db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []map[string]any
	for rs.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = normalizeSQLValue(holders[i])
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}
	return out, cols, nil
}

func normalizeSQLValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return t
	}
}

func runLocalPipeline(args []string, stdout io.Writer, globals globalOptions) error {
	if len(args) == 0 {
		return errors.New("usage: topline local pipeline snapshot|stale [--pipeline-id ID] [--days N]")
	}
	flags, err := parseFlags(args[1:])
	if err != nil {
		return err
	}
	dbPath, err := resolveDBPath(flags)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	switch args[0] {
	case "snapshot":
		return runLocalPipelineSnapshot(db, flags, stdout, globals)
	case "stale":
		return runLocalPipelineStale(db, flags, stdout, globals)
	default:
		return fmt.Errorf("unknown local pipeline subcommand %q", args[0])
	}
}

func runLocalPipelineSnapshot(db *sql.DB, flags map[string]string, stdout io.Writer, globals globalOptions) error {
	pipelineID := strings.TrimSpace(flags["pipelineId"])
	q := `
SELECT
  p.id   AS pipeline_id,
  p.name AS pipeline,
  s.id   AS stage_id,
  s.name AS stage,
  COUNT(o.id)                       AS open_count,
  COALESCE(SUM(o.monetary_value),0) AS open_value
FROM pipelines p
JOIN pipeline_stages s ON s.pipeline_id = p.id
LEFT JOIN opportunities o
  ON o.pipeline_id = p.id
 AND o.pipeline_stage_id = s.id
 AND (o.status IS NULL OR o.status = 'open')
`
	args := []any{}
	if pipelineID != "" {
		q += " WHERE p.id = ?\n"
		args = append(args, pipelineID)
	}
	q += " GROUP BY p.id, s.id ORDER BY p.name, s.name"
	rows, cols, err := executeLocalSQLArgs(context.Background(), db, q, args...)
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, map[string]any{
		"view":    "pipeline_snapshot",
		"columns": cols,
		"rows":    rows,
		"count":   len(rows),
	}, globals.MaskPII)
}

func runLocalPipelineStale(db *sql.DB, flags map[string]string, stdout io.Writer, globals globalOptions) error {
	days := 14
	if d := strings.TrimSpace(flags["days"]); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid --days %q", d)
		}
		days = n
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	pipelineID := strings.TrimSpace(flags["pipelineId"])
	q := `
SELECT
  o.id,
  o.name,
  o.status,
  o.monetary_value,
  o.pipeline_id,
  o.pipeline_stage_id,
  s.name AS stage,
  o.updated_at
FROM opportunities o
LEFT JOIN pipeline_stages s ON s.id = o.pipeline_stage_id
WHERE (o.status IS NULL OR o.status = 'open')
  AND (o.updated_at IS NULL OR o.updated_at < ?)
`
	args := []any{cutoff}
	if pipelineID != "" {
		q += " AND o.pipeline_id = ?\n"
		args = append(args, pipelineID)
	}
	q += " ORDER BY o.updated_at ASC"
	rows, cols, err := executeLocalSQLArgs(context.Background(), db, q, args...)
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, map[string]any{
		"view":    "pipeline_stale",
		"days":    days,
		"cutoff":  cutoff,
		"columns": cols,
		"rows":    rows,
		"count":   len(rows),
	}, globals.MaskPII)
}

func executeLocalSQLArgs(ctx context.Context, db *sql.DB, sqlText string, args ...any) ([]map[string]any, []string, error) {
	rs, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []map[string]any
	for rs.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = normalizeSQLValue(holders[i])
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}
	return out, cols, nil
}

func runLocalStatus(args []string, stdout io.Writer, globals globalOptions) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	dbPath, err := resolveDBPath(flags)
	if err != nil {
		return err
	}
	info := map[string]any{"db": dbPath}
	if _, err := os.Stat(dbPath); err != nil {
		info["initialized"] = false
		return output.WriteJSON(stdout, info, globals.MaskPII)
	}
	info["initialized"] = true
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	tables := []string{"contacts", "opportunities", "pipelines", "pipeline_stages", "conversations", "messages", "tasks", "notes"}
	counts := map[string]int{}
	for _, t := range tables {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err == nil {
			counts[t] = n
		}
	}
	info["counts"] = counts
	var lastSync sql.NullString
	if err := db.QueryRow(`SELECT value FROM sync_state WHERE key='last_sync_at'`).Scan(&lastSync); err == nil && lastSync.Valid {
		info["last_sync_at"] = lastSync.String
	}
	return output.WriteJSON(stdout, info, globals.MaskPII)
}

// suppress unused warning when no JSON marshalling needed elsewhere
var _ = json.Marshal

// runLocalDeal dispatches `topline local deal …`. First subcommand: `brief`.
func runLocalDeal(args []string, stdout io.Writer, globals globalOptions) error {
	if len(args) == 0 {
		return errors.New("usage: topline local deal brief --opportunity-id OPP [--messages N]")
	}
	switch args[0] {
	case "brief":
		return runLocalDealBrief(args[1:], stdout, globals)
	default:
		return fmt.Errorf("unknown local deal subcommand %q", args[0])
	}
}

// runLocalDealBrief returns a single-call snapshot of a deal for an agent or
// human: the opportunity row, its pipeline + stage names, the linked contact,
// the most recent messages on any of that contact's conversations, open
// tasks, and notes — all from the local SQLite mirror. Zero network calls.
func runLocalDealBrief(args []string, stdout io.Writer, globals globalOptions) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	oppID := strings.TrimSpace(flags["opportunityId"])
	if oppID == "" {
		return errors.New("usage: topline local deal brief --opportunity-id OPP [--messages N]")
	}
	msgLimit := 20
	if v := strings.TrimSpace(flags["messages"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid --messages %q", v)
		}
		msgLimit = n
	}
	dbPath, err := resolveDBPath(flags)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()

	opp, err := singleRow(ctx, db, `
SELECT o.id, o.name, o.status, o.monetary_value, o.pipeline_id, o.pipeline_stage_id,
       o.contact_id, o.updated_at,
       p.name AS pipeline, s.name AS stage
FROM opportunities o
LEFT JOIN pipelines p ON p.id = o.pipeline_id
LEFT JOIN pipeline_stages s ON s.id = o.pipeline_stage_id
WHERE o.id = ?`, oppID)
	if err != nil {
		return err
	}
	if opp == nil {
		return output.WriteJSON(stdout, map[string]any{
			"opportunityId": oppID,
			"found":         false,
			"hint":          "run `topline local sync` then retry",
		}, globals.MaskPII)
	}

	contactID, _ := opp["contact_id"].(string)
	var contact map[string]any
	if contactID != "" {
		contact, _ = singleRow(ctx, db, `SELECT id, name, email, phone, updated_at FROM contacts WHERE id = ?`, contactID)
	}

	var messages []map[string]any
	if contactID != "" {
		rows, _, mErr := executeLocalSQLArgs(ctx, db, `
SELECT m.id, m.conversation_id, m.type, m.direction, m.created_at,
       substr(m.raw_json, 1, 4000) AS raw_json
FROM messages m
WHERE m.contact_id = ?
ORDER BY COALESCE(m.created_at, '') DESC
LIMIT ?`, contactID, msgLimit)
		if mErr == nil {
			messages = rows
		}
	}

	var tasks []map[string]any
	if contactID != "" {
		rows, _, tErr := executeLocalSQLArgs(ctx, db, `
SELECT id, title, due_at, completed
FROM tasks
WHERE contact_id = ?
ORDER BY COALESCE(due_at, '') ASC`, contactID)
		if tErr == nil {
			tasks = rows
		}
	}

	var notes []map[string]any
	if contactID != "" {
		rows, _, nErr := executeLocalSQLArgs(ctx, db, `
SELECT id, body, created_at
FROM notes
WHERE contact_id = ?
ORDER BY COALESCE(created_at, '') DESC`, contactID)
		if nErr == nil {
			notes = rows
		}
	}

	out := map[string]any{
		"view":        "deal_brief",
		"found":       true,
		"opportunity": opp,
		"contact":     contact,
		"messages":    messages,
		"tasks":       tasks,
		"notes":       notes,
		"counts": map[string]int{
			"messages": len(messages),
			"tasks":    len(tasks),
			"notes":    len(notes),
		},
	}
	return output.WriteJSON(stdout, out, globals.MaskPII)
}

func singleRow(ctx context.Context, db *sql.DB, sqlText string, args ...any) (map[string]any, error) {
	rows, _, err := executeLocalSQLArgs(ctx, db, sqlText, args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
