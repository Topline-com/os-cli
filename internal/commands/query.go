package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

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
