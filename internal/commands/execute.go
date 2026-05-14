package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Topline-com/os-cli/internal/output"
	localsync "github.com/Topline-com/os-cli/internal/sync"
	"github.com/Topline-com/os-cli/internal/topline"
)

type globalOptions struct {
	JSON       bool
	Agent      bool
	MaskPII    bool
	BaseURL    string
	LocationID string
}

func Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp(stdout)
		return nil
	}
	globals, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		printHelp(stdout)
		return nil
	}
	if rest[0] == "setup-check" || rest[0] == "topline_setup_check" {
		return runSetupCheck(stdout, globals)
	}
	if len(rest) >= 2 && rest[0] == "raw" && rest[1] == "request" {
		return runRawRequest(rest[2:], stdout, globals)
	}
	if len(rest) >= 2 && rest[0] == "pipeline" && rest[1] == "audit" {
		return runPipelineAudit(rest[2:], stdout, globals)
	}
	if len(rest) >= 1 && rest[0] == "query" {
		return runQueryCommand(rest[1:], stdout, globals)
	}
	if len(rest) >= 1 && rest[0] == "local" {
		return runLocalCommand(rest[1:], stdout, globals)
	}
	if len(rest) >= 2 && rest[0] == "sync" && rest[1] == "init" {
		flags, err := parseFlags(rest[2:])
		if err != nil {
			return err
		}
		path := flags["db"]
		if path == "" {
			path = "topline.db"
		}
		if err := localsync.InitDB(path); err != nil {
			return err
		}
		return output.WriteJSON(stdout, map[string]any{"ok": true, "db": path}, globals.MaskPII)
	}
	spec, consumed, err := findSpec(rest)
	if err != nil {
		return err
	}
	flags, err := parseFlags(rest[consumed:])
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
	flags = spec.WithLocation(flags, cfg.LocationID)
	req, err := spec.Resolve(flags)
	if err != nil {
		return err
	}
	client := topline.NewClient(cfg)
	var result any
	if err := client.Do(context.Background(), req, &result); err != nil {
		return err
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

func parseGlobal(args []string) (globalOptions, []string, error) {
	var g globalOptions
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			g.JSON = true
		case a == "--agent":
			g.Agent = true
			g.MaskPII = true
		case a == "--mask-pii":
			g.MaskPII = true
		case a == "--base-url" || a == "--location-id":
			if i+1 >= len(args) {
				return g, nil, fmt.Errorf("%s needs a value", a)
			}
			if a == "--base-url" {
				g.BaseURL = args[i+1]
			} else {
				g.LocationID = args[i+1]
			}
			i++
		case strings.HasPrefix(a, "--base-url="):
			g.BaseURL = strings.TrimPrefix(a, "--base-url=")
		case strings.HasPrefix(a, "--location-id="):
			g.LocationID = strings.TrimPrefix(a, "--location-id=")
		default:
			rest = append(rest, a)
		}
	}
	return g, rest, nil
}

func parseFlags(args []string) (map[string]string, error) {
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			return nil, fmt.Errorf("unexpected positional argument %q; use --flag value", a)
		}
		keyVal := strings.TrimPrefix(a, "--")
		if strings.Contains(keyVal, "=") {
			parts := strings.SplitN(keyVal, "=", 2)
			flags[camelFlag(parts[0])] = parts[1]
			continue
		}
		key := camelFlag(keyVal)
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			flags[key] = args[i+1]
			i++
		} else {
			flags[key] = "true"
		}
	}
	return flags, nil
}

func camelFlag(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func findSpec(args []string) (Spec, int, error) {
	byCommand := SpecsByCommand()
	max := 4
	if len(args) < max {
		max = len(args)
	}
	for n := max; n >= 1; n-- {
		key := strings.Join(args[:n], " ")
		if spec, ok := byCommand[key]; ok && spec.ToolName != "topline_request" && spec.ToolName != "topline_setup_check" {
			return spec, n, nil
		}
	}
	return Spec{}, 0, fmt.Errorf("unknown command %q", strings.Join(args, " "))
}

func runRawRequest(args []string, stdout io.Writer, globals globalOptions) error {
	if len(args) < 2 {
		return errors.New("usage: topline raw request METHOD /path [--query '{...}'] [--body '{...}'] [--inject-location false]")
	}
	method, path := strings.ToUpper(args[0]), args[1]
	flags, err := parseFlags(args[2:])
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
	query := map[string]string{}
	if flags["query"] != "" {
		var q map[string]any
		if err := json.Unmarshal([]byte(flags["query"]), &q); err != nil {
			return err
		}
		for k, v := range q {
			query[k] = fmt.Sprint(v)
		}
	}
	if flags["injectLocation"] != "false" && query["locationId"] == "" {
		query["locationId"] = cfg.LocationID
	}
	body := map[string]any(nil)
	if flags["body"] != "" {
		if err := json.Unmarshal([]byte(flags["body"]), &body); err != nil {
			return err
		}
	}
	var result any
	if err := topline.NewClient(cfg).Do(context.Background(), topline.Request{Method: method, Path: path, Query: query, Body: body}, &result); err != nil {
		return err
	}
	return output.WriteJSON(stdout, result, globals.MaskPII)
}

func printHelp(w io.Writer) {
	commands := make([]string, 0, len(Specs()))
	for _, spec := range Specs() {
		if len(spec.Command) > 0 && spec.ToolName != "topline_request" && spec.ToolName != "topline_setup_check" {
			commands = append(commands, strings.Join(spec.Command, " "))
		}
	}
	sort.Strings(commands)
	_, _ = fmt.Fprintln(w, "Topline OS CLI")
	_, _ = fmt.Fprintln(w, "\nUsage:")
	_, _ = fmt.Fprintln(w, "  topline <command> [--flag value]")
	_, _ = fmt.Fprintln(w, "  topline --agent pipeline audit --pipeline-id PIPE --since this-week-et")
	_, _ = fmt.Fprintln(w, "  topline --agent query sql --sql 'SELECT COUNT(*) AS n FROM opportunities'")
	_, _ = fmt.Fprintln(w, "  topline raw request GET /contacts/ --query '{\"limit\":1}'")
	_, _ = fmt.Fprintln(w, "\nAgent-native commands:")
	_, _ = fmt.Fprintln(w, "  pipeline audit --pipeline-id PIPE --since YYYY-MM-DD|this-week-et [--concurrency 8] [--skip-activity]")
	_, _ = fmt.Fprintln(w, "  query schema | catalog | explain --tables a,b | sql --sql SELECT...")
	_, _ = fmt.Fprintln(w, "  local sync                                 # one-token native sync into ~/.topline/state.db")
	_, _ = fmt.Fprintln(w, "  local status | sql --sql ... | pipeline snapshot | pipeline stale --days 14")
	_, _ = fmt.Fprintln(w, "  sync init --db topline.db")
	_, _ = fmt.Fprintln(w, "\nParity commands:")
	for _, cmd := range commands {
		_, _ = fmt.Fprintf(w, "  %s\n", cmd)
	}
}
