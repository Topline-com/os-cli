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
	_, _ = fmt.Fprintln(w, "  topline query schema")
	_, _ = fmt.Fprintln(w, "  topline query catalog")
	_, _ = fmt.Fprintln(w, "  topline query explain --tables contacts,opportunities")
	_, _ = fmt.Fprintln(w, "  topline query sql --sql 'SELECT COUNT(*) AS n FROM contacts'")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Environment:")
	_, _ = fmt.Fprintln(w, "  TOPLINE_QUERY_TOKEN       Connection-bound token from https://os-mcp.topline.com/connect")
	_, _ = fmt.Fprintln(w, "  TOPLINE_QUERY_BASE_URL    Defaults to https://os-mcp.topline.com")
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
