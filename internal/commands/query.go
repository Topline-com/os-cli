package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Topline-com/os-cli/internal/output"
	"github.com/Topline-com/os-cli/internal/topline"
)

func runQueryCommand(args []string, stdout io.Writer, globals globalOptions) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printQueryHelp(stdout)
		return nil
	}

	subcommand := args[0]
	flags, err := parseFlags(args[1:])
	if err != nil {
		return err
	}
	if subcommand == "doctor" {
		return runQueryDoctor(flags, stdout, globals)
	}
	cfg, err := topline.LoadQueryConfig()
	if err != nil {
		return err
	}
	if flags["url"] != "" {
		cfg.BaseURL = flags["url"]
	}
	client := topline.NewQueryClient(cfg)
	ctx := context.Background()

	var result any
	switch subcommand {
	case "schema", "describe", "overview":
		err = client.Get(ctx, "/query/api/get-overview", nil, &result)
	case "catalog":
		err = client.Get(ctx, "/query/api/catalog", nil, &result)
	case "explain", "tables":
		tables := splitCSV(firstNonEmptyQueryFlag(flags["tables"], flags["table"]))
		if len(tables) == 0 {
			return errors.New("usage: topline query explain --tables contacts,opportunities")
		}
		values := url.Values{}
		for _, table := range tables {
			values.Add("table", table)
		}
		err = client.Get(ctx, "/query/api/explain-tables", values, &result)
	case "sql", "execute":
		sqlText := strings.TrimSpace(flags["sql"])
		if sqlText == "" && flags["file"] != "" {
			b, readErr := os.ReadFile(flags["file"])
			if readErr != nil {
				return readErr
			}
			sqlText = strings.TrimSpace(string(b))
		}
		if sqlText == "" {
			return errors.New("usage: topline query sql --sql 'SELECT COUNT(*) FROM contacts' [--url https://os-mcp.topline.com]")
		}
		err = client.PostJSON(ctx, "/query/api/execute-sql", map[string]any{"sql": sqlText}, &result)
	case "freshness":
		return runQueryFreshness(ctx, client, stdout, globals)
	case "snapshot":
		return runQuerySnapshot(ctx, client, flags, stdout, globals)
	case "audit":
		return runQueryAudit(ctx, client, flags, stdout, globals)
	default:
		return fmt.Errorf("unknown query command %q; try topline query help", subcommand)
	}
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

func printQueryHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Topline SQL/query commands")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  topline query doctor")
	_, _ = fmt.Fprintln(w, "  topline query schema")
	_, _ = fmt.Fprintln(w, "  topline query catalog")
	_, _ = fmt.Fprintln(w, "  topline query explain --tables contacts,opportunities")
	_, _ = fmt.Fprintln(w, "  topline query sql --sql 'SELECT COUNT(*) AS n FROM contacts'")
	_, _ = fmt.Fprintln(w, "  topline query freshness")
	_, _ = fmt.Fprintln(w, "  topline query snapshot --pipeline <id_or_name>")
	_, _ = fmt.Fprintln(w, "  topline query audit --pipeline <id_or_name> [--since this-week-et] [--status open]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Composite commands (Phase 3): one CLI call wraps the warehouse views")
	_, _ = fmt.Fprintln(w, "shipped in Topline-com/os-mcp#1. Use these for pipeline audits instead")
	_, _ = fmt.Fprintln(w, "of hand-stitched CTEs.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "--pipeline accepts an opaque 20-char ID (e.g. CLUy1QapsrEeBiNrmQiL)")
	_, _ = fmt.Fprintln(w, "or a fuzzy name (e.g. 'flex triage'). On ambiguous or missing names")
	_, _ = fmt.Fprintln(w, "the CLI errors with the list of available pipelines.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Environment:")
	_, _ = fmt.Fprintln(w, "  TOPLINE_QUERY_TOKEN       Connection-bound token from https://os-mcp.topline.com/connect")
	_, _ = fmt.Fprintln(w, "  TOPLINE_QUERY_BASE_URL    Defaults to https://os-mcp.topline.com")
}

// expectedQueryTables are the warehouse tables every SQL-first agent flow assumes
// are reachable. `query doctor` reports presence so agents can decide SQL vs REST
// without guessing.
var expectedQueryTables = []string{
	"contacts",
	"opportunities",
	"messages",
	"pipelines",
	"pipeline_stages",
	"call_events",
	"appointments",
	"conversations",
}

type queryDoctorReport struct {
	QueryTokenPresent bool            `json:"queryTokenPresent"`
	TokenSourceEnvVar string          `json:"tokenSourceEnvVar,omitempty"`
	RawPITRejected    bool            `json:"rawPitRejected"`
	BaseURL           string          `json:"baseUrl"`
	SchemaReachable   bool            `json:"schemaReachable"`
	SchemaError       string          `json:"schemaError,omitempty"`
	TableCount        int             `json:"tableCount"`
	ExpectedTables    map[string]bool `json:"expectedTables"`
	MissingTables     []string        `json:"missingTables,omitempty"`
	Recommendation    string          `json:"recommendation"`
}

func runQueryDoctor(flags map[string]string, stdout io.Writer, globals globalOptions) error {
	status := topline.InspectQueryEnv()
	if flags["url"] != "" {
		status.BaseURL = flags["url"]
	}

	report := queryDoctorReport{
		QueryTokenPresent: status.TokenPresent,
		TokenSourceEnvVar: status.SourceEnvVar,
		RawPITRejected:    status.RawPITToken,
		BaseURL:           status.BaseURL,
		ExpectedTables:    map[string]bool{},
	}

	for _, t := range expectedQueryTables {
		report.ExpectedTables[t] = false
	}

	switch {
	case !status.TokenPresent:
		report.Recommendation = "Set TOPLINE_QUERY_TOKEN to a connection-bound token from https://os-mcp.topline.com/connect, or fall back to REST commands (e.g. `topline pipeline audit`) until SQL is wired."
		return output.WriteJSON(stdout, report, globals.MaskPII)
	case status.RawPITToken:
		report.Recommendation = "TOPLINE_QUERY_TOKEN looks like a raw PIT (pit-...). SQL surface requires a connection-bound token; generate one at https://os-mcp.topline.com/connect."
		return output.WriteJSON(stdout, report, globals.MaskPII)
	}

	client := topline.NewQueryClient(topline.QueryConfig{BaseURL: status.BaseURL, Token: status.Token})
	ctx := context.Background()

	var schema any
	if err := client.Get(ctx, "/query/api/get-overview", nil, &schema); err != nil {
		report.SchemaReachable = false
		report.SchemaError = err.Error()
		report.Recommendation = "SQL endpoint unreachable. Verify TOPLINE_QUERY_BASE_URL and that the connection-bound token is still valid; fall back to REST until resolved."
		return output.WriteJSON(stdout, report, globals.MaskPII)
	}

	report.SchemaReachable = true
	tables := extractTableNames(schema)
	report.TableCount = len(tables)
	for _, name := range tables {
		if _, ok := report.ExpectedTables[name]; ok {
			report.ExpectedTables[name] = true
		}
	}
	for _, name := range expectedQueryTables {
		if !report.ExpectedTables[name] {
			report.MissingTables = append(report.MissingTables, name)
		}
	}
	if len(report.MissingTables) == 0 {
		report.Recommendation = "SQL analytics ready. Prefer warehouse SQL for pipeline/activity questions; REST only for live writes."
	} else {
		report.Recommendation = fmt.Sprintf("SQL reachable but missing expected tables (%s). Treat the gap as an os-mcp coverage bug rather than silently falling back to REST.", strings.Join(report.MissingTables, ", "))
	}
	return output.WriteJSON(stdout, report, globals.MaskPII)
}

func extractTableNames(schema any) []string {
	root, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := root["tables"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		switch t := entry.(type) {
		case string:
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		case map[string]any:
			if name, ok := t["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstNonEmptyQueryFlag(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// executeSQL runs one SQL against /query/api/execute-sql and returns the parsed
// response shape as map[string]any. Used by the composite commands to wrap the
// warehouse views shipped in Topline-com/os-mcp#1.
func executeSQL(ctx context.Context, client *topline.QueryClient, sql string) (map[string]any, error) {
	var raw any
	if err := client.PostJSON(ctx, "/query/api/execute-sql", map[string]any{"sql": sql}, &raw); err != nil {
		return nil, err
	}
	if m, ok := raw.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{"raw": raw}, nil
}

func runQueryFreshness(ctx context.Context, client *topline.QueryClient, stdout io.Writer, globals globalOptions) error {
	const sqlText = "SELECT table_name, row_count, last_synced_at, lag_seconds FROM warehouse_freshness ORDER BY table_name"
	result, err := executeSQL(ctx, client, sqlText)
	if err != nil {
		return err
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

func runQuerySnapshot(ctx context.Context, client *topline.QueryClient, flags map[string]string, stdout io.Writer, globals globalOptions) error {
	rawPipeline := strings.TrimSpace(firstNonEmptyQueryFlag(flags["pipeline"], flags["pipelineId"], flags["pipelineID"]))
	if rawPipeline == "" {
		return errors.New("usage: topline query snapshot --pipeline <pipeline_id_or_name>")
	}
	resolution, err := resolvePipelineID(ctx, client, rawPipeline)
	if err != nil {
		return err
	}
	pipelineID := resolution.MatchedID
	status := strings.TrimSpace(flags["status"])
	if status == "" {
		status = "open"
	}
	sqlText := fmt.Sprintf(
		"SELECT pipeline_id, pipeline_name, pipeline_stage_id, stage_name, stage_position, opportunity_status, "+
			"opportunity_count, pipeline_value, CAST(avg_days_in_stage AS INTEGER) AS avg_days_in_stage "+
			"FROM pipeline_snapshot WHERE pipeline_id = %s%s ORDER BY stage_position",
		sqlString(pipelineID), queryStatusClause("opportunity_status", status),
	)
	rows, err := executeSQL(ctx, client, sqlText)
	if err != nil {
		return err
	}
	result := map[string]any{
		"pipelineId":         pipelineID,
		"pipelineResolution": resolution,
		"status":             status,
		"snapshot":           rows,
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

func runQueryAudit(ctx context.Context, client *topline.QueryClient, flags map[string]string, stdout io.Writer, globals globalOptions) error {
	rawPipeline := strings.TrimSpace(firstNonEmptyQueryFlag(flags["pipeline"], flags["pipelineId"], flags["pipelineID"]))
	if rawPipeline == "" {
		return errors.New("usage: topline query audit --pipeline <pipeline_id_or_name> [--since this-week-et] [--status open]")
	}
	resolution, err := resolvePipelineID(ctx, client, rawPipeline)
	if err != nil {
		return err
	}
	pipelineID := resolution.MatchedID
	start, err := parseAuditTime(flags["since"], time.Now().AddDate(0, 0, -7))
	if err != nil {
		return err
	}
	end, err := parseAuditTime(flags["until"], time.Now())
	if err != nil {
		return err
	}
	since := start.UTC().Format(time.RFC3339)
	until := end.UTC().Format(time.RFC3339)
	status := strings.TrimSpace(flags["status"])
	if status == "" {
		status = "open"
	}
	dealLimit := parsePositiveInt(flags["limit"], 25)
	if dealLimit > 100 {
		dealLimit = 100
	}
	statusClause := queryStatusClause("opportunity_status", status)

	freshnessSQL := "SELECT table_name, row_count, last_synced_at, lag_seconds FROM warehouse_freshness ORDER BY table_name"
	snapshotSQL := fmt.Sprintf(
		"SELECT pipeline_id, pipeline_name, pipeline_stage_id, stage_name, stage_position, opportunity_status, "+
			"opportunity_count, pipeline_value, CAST(avg_days_in_stage AS INTEGER) AS avg_days_in_stage "+
			"FROM pipeline_snapshot WHERE pipeline_id = %s%s ORDER BY stage_position",
		sqlString(pipelineID), statusClause,
	)
	activitySQL := fmt.Sprintf(
		"SELECT activity_class, direction, COUNT(DISTINCT source_id) AS unique_touches, "+
			"COUNT(DISTINCT opportunity_id) AS opportunities_touched, COUNT(DISTINCT contact_id) AS contacts_touched, "+
			"MIN(event_at) AS first_touch, MAX(event_at) AS last_touch "+
			"FROM pipeline_activity_window WHERE pipeline_id = %s%s AND event_at >= %s AND event_at <= %s "+
			"GROUP BY activity_class, direction ORDER BY unique_touches DESC",
		sqlString(pipelineID), statusClause, sqlString(since), sqlString(until),
	)
	dealsSQL := fmt.Sprintf(
		"SELECT opportunity_id, opportunity_name, contact_id, pipeline_stage_id, owner_user_id, ROUND(monetary_value, 2) AS monetary_value, "+
			"COUNT(DISTINCT source_id) AS unique_touches, "+
			"COUNT(DISTINCT CASE WHEN activity_class = 'message' THEN source_id END) AS message_touches, "+
			"COUNT(DISTINCT CASE WHEN activity_class = 'call' THEN source_id END) AS call_touches, "+
			"COUNT(DISTINCT CASE WHEN activity_class = 'appointment' THEN source_id END) AS appointment_touches, "+
			"COUNT(DISTINCT CASE WHEN direction = 'inbound' THEN source_id END) AS inbound_touches, "+
			"COUNT(DISTINCT CASE WHEN direction = 'outbound' THEN source_id END) AS outbound_touches, "+
			"MIN(event_at) AS first_touch, MAX(event_at) AS last_touch "+
			"FROM pipeline_activity_window WHERE pipeline_id = %s%s AND event_at >= %s AND event_at <= %s "+
			"GROUP BY opportunity_id, opportunity_name, contact_id, pipeline_stage_id, owner_user_id, monetary_value "+
			"ORDER BY unique_touches DESC, monetary_value DESC LIMIT %d",
		sqlString(pipelineID), statusClause, sqlString(since), sqlString(until), dealLimit,
	)
	movementSQL := fmt.Sprintf(
		"SELECT opportunity_id, opportunity_name, contact_id, pipeline_stage_id, stage_name, opportunity_status, monetary_value, "+
			"last_movement_at, last_movement_kind "+
			"FROM pipeline_movement_window WHERE pipeline_id = %s%s AND last_movement_at >= %s AND last_movement_at <= %s "+
			"ORDER BY last_movement_at DESC",
		sqlString(pipelineID), statusClause, sqlString(since), sqlString(until),
	)

	freshness, err := executeSQL(ctx, client, freshnessSQL)
	if err != nil {
		return err
	}
	snapshot, err := executeSQL(ctx, client, snapshotSQL)
	if err != nil {
		return err
	}
	activity, err := executeSQL(ctx, client, activitySQL)
	if err != nil {
		return err
	}
	deals, err := executeSQL(ctx, client, dealsSQL)
	if err != nil {
		return err
	}
	movement, err := executeSQL(ctx, client, movementSQL)
	if err != nil {
		return err
	}

	result := map[string]any{
		"pipelineId":         pipelineID,
		"pipelineResolution": resolution,
		"window":             map[string]string{"since": since, "until": until},
		"status":             status,
		"freshness":          freshness,
		"snapshot":           snapshot,
		"activity":           activity,
		"deals":              deals,
		"movement":           movement,
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

// pipelineIDPattern matches an opaque 20-char alphanumeric warehouse pipeline ID
// (e.g. "CLUy1QapsrEeBiNrmQiL", "bna6e9DoPgRchNsjeYS3"). Names are typically
// multi-word with spaces/punctuation, so this regex distinguishes the two
// without an extra HTTP round-trip.
var pipelineIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{20}$`)

// PipelineResolution surfaces how `--pipeline` was interpreted so the agent
// can see whether it passed an opaque ID directly or matched a name.
type PipelineResolution struct {
	Input        string `json:"input"`
	MatchedID    string `json:"matchedId"`
	MatchedName  string `json:"matchedName,omitempty"`
	MatchedBy    string `json:"matchedBy"` // "id" | "name"
	CandidateCnt int    `json:"candidateCount,omitempty"`
}

// resolvePipelineID converts the --pipeline flag value into an opaque pipeline
// ID. If the input already matches the opaque-ID shape it is returned as-is;
// otherwise a case-insensitive LIKE lookup against the `pipelines` table is
// run. On 0 or >1 matches the error message lists candidate pipelines so the
// caller (agent or human) can disambiguate without dropping to raw SQL.
func resolvePipelineID(ctx context.Context, client *topline.QueryClient, input string) (PipelineResolution, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return PipelineResolution{}, errors.New("--pipeline is required (id or name)")
	}
	if pipelineIDPattern.MatchString(input) {
		return PipelineResolution{Input: input, MatchedID: input, MatchedBy: "id"}, nil
	}
	tokens := strings.Fields(strings.ToLower(input))
	if len(tokens) == 0 {
		return PipelineResolution{}, errors.New("--pipeline value is empty after whitespace trim")
	}
	clauses := make([]string, len(tokens))
	for i, tok := range tokens {
		clauses[i] = fmt.Sprintf("LOWER(name) LIKE %s", sqlString("%"+tok+"%"))
	}
	matchSQL := fmt.Sprintf(
		"SELECT id, name FROM pipelines WHERE %s ORDER BY name",
		strings.Join(clauses, " AND "),
	)
	matches, err := selectIDNamePairs(ctx, client, matchSQL)
	if err != nil {
		return PipelineResolution{}, fmt.Errorf("pipeline name resolution failed for %q: %w", input, err)
	}
	switch len(matches) {
	case 1:
		return PipelineResolution{
			Input:       input,
			MatchedID:   matches[0].ID,
			MatchedName: matches[0].Name,
			MatchedBy:   "name",
		}, nil
	case 0:
		all, listErr := selectIDNamePairs(ctx, client, "SELECT id, name FROM pipelines ORDER BY name")
		if listErr != nil {
			return PipelineResolution{}, fmt.Errorf("no pipeline matched %q (and listing failed: %v)", input, listErr)
		}
		return PipelineResolution{}, fmt.Errorf(
			"no pipeline matched %q. Available pipelines: %s",
			input, formatPipelineList(all),
		)
	default:
		return PipelineResolution{}, fmt.Errorf(
			"pipeline %q is ambiguous (%d matches). Pass the opaque id or a more specific name. Candidates: %s",
			input, len(matches), formatPipelineList(matches),
		)
	}
}

type idNamePair struct {
	ID   string
	Name string
}

func selectIDNamePairs(ctx context.Context, client *topline.QueryClient, sql string) ([]idNamePair, error) {
	raw, err := executeSQL(ctx, client, sql)
	if err != nil {
		return nil, err
	}
	columns := stringSliceFromAny(raw["columns"])
	idIdx, nameIdx := -1, -1
	for i, col := range columns {
		switch strings.ToLower(col) {
		case "id":
			idIdx = i
		case "name":
			nameIdx = i
		}
	}
	if idIdx == -1 || nameIdx == -1 {
		return nil, fmt.Errorf("pipeline lookup response missing id/name columns (got %v)", columns)
	}
	rowsAny, _ := raw["rows"].([]any)
	out := make([]idNamePair, 0, len(rowsAny))
	for _, r := range rowsAny {
		var id, name string
		switch row := r.(type) {
		case map[string]any:
			// Hosted warehouse returns rows as column-keyed objects, e.g.
			// {"id": "...", "name": "..."}. Use column names directly.
			id, _ = row["id"].(string)
			name, _ = row["name"].(string)
		case []any:
			// Some deployments / tests return rows as positional arrays
			// aligned with the `columns` order.
			if len(row) <= idIdx || len(row) <= nameIdx {
				continue
			}
			id, _ = row[idIdx].(string)
			name, _ = row[nameIdx].(string)
		default:
			continue
		}
		id = strings.TrimSpace(id)
		name = strings.TrimSpace(name)
		if id == "" {
			continue
		}
		out = append(out, idNamePair{ID: id, Name: name})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func stringSliceFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		if s, ok := entry.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func formatPipelineList(pairs []idNamePair) string {
	if len(pairs) == 0 {
		return "(none found)"
	}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s (%s)", p.Name, p.ID)
	}
	return strings.Join(parts, ", ")
}

// sqlString quotes a value as a SQLite single-quoted literal. Used because the
// query API doesn't yet accept bind parameters — pipeline IDs are caller-controlled
// CRM identifiers, not user input, but we still escape embedded single-quotes.
func sqlString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func queryStatusClause(column string, status string) string {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" || strings.EqualFold(trimmed, "all") || strings.EqualFold(trimmed, "any") {
		return ""
	}
	return fmt.Sprintf(" AND %s = %s", column, sqlString(trimmed))
}
