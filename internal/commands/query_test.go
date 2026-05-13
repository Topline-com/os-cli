package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuerySchemaUsesHTTPQueryAPI(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "signed-query-token")

	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tables":[{"name":"opportunities","row_count":242}]}`))
	}))
	defer server.Close()
	t.Setenv("TOPLINE_QUERY_BASE_URL", server.URL)

	var stdout bytes.Buffer
	if err := Execute([]string{"--agent", "query", "schema"}, &stdout, io.Discard); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if gotPath != "/query/api/get-overview" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer signed-query-token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if !strings.Contains(stdout.String(), "opportunities") || !strings.Contains(stdout.String(), "242") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestQuerySQLPostsSQLToExecuteEndpoint(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "signed-query-token")

	var gotSQL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/api/execute-sql" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("Content-Type = %q", ct)
		}
		var body struct {
			SQL string `json:"sql"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		gotSQL = body.SQL
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"columns":["n"],"rows":[[242]],"elapsed_ms":3,"truncated":false,"effective_limit":5000,"rewritten_sql":"SELECT COUNT(*) AS n FROM opportunities LIMIT 5000"}`))
	}))
	defer server.Close()
	t.Setenv("TOPLINE_QUERY_BASE_URL", server.URL)

	var stdout bytes.Buffer
	err := Execute([]string{"query", "sql", "--sql", "SELECT COUNT(*) AS n FROM opportunities"}, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if gotSQL != "SELECT COUNT(*) AS n FROM opportunities" {
		t.Fatalf("sql = %q", gotSQL)
	}
	if !strings.Contains(stdout.String(), "242") || !strings.Contains(stdout.String(), "rewritten_sql") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestQueryExplainBuildsRepeatedTableParams(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "signed-query-token")

	var gotTables []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/api/explain-tables" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotTables = r.URL.Query()["table"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tables":[]}`))
	}))
	defer server.Close()
	t.Setenv("TOPLINE_QUERY_BASE_URL", server.URL)

	var stdout bytes.Buffer
	err := Execute([]string{"query", "explain", "--tables", "contacts,opportunities"}, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Join(gotTables, ",") != "contacts,opportunities" {
		t.Fatalf("tables = %#v", gotTables)
	}
}

func TestQueryRejectsRawPITToken(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "pit-example-token")
	t.Setenv("TOPLINE_QUERY_BASE_URL", "http://127.0.0.1:1")

	var stdout bytes.Buffer
	err := Execute([]string{"query", "schema"}, &stdout, io.Discard)
	if err == nil {
		t.Fatal("expected raw PIT token error")
	}
	if !strings.Contains(err.Error(), "connection-bound") || !strings.Contains(err.Error(), "not a raw PIT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func decodeDoctor(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode doctor output: %v\nraw: %s", err, raw)
	}
	return out
}

func TestQueryDoctorReportsMissingToken(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "")
	t.Setenv("TOPLINE_MCP_ACCESS_TOKEN", "")
	t.Setenv("TOPLINE_MCP_TOKEN", "")
	t.Setenv("TOPLINE_QUERY_BASE_URL", "")

	var stdout bytes.Buffer
	if err := Execute([]string{"query", "doctor"}, &stdout, io.Discard); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := decodeDoctor(t, stdout.String())
	if got, _ := out["queryTokenPresent"].(bool); got {
		t.Fatalf("queryTokenPresent = true, want false")
	}
	if got, _ := out["schemaReachable"].(bool); got {
		t.Fatalf("schemaReachable should be false when token is missing")
	}
	rec, _ := out["recommendation"].(string)
	if !strings.Contains(rec, "TOPLINE_QUERY_TOKEN") {
		t.Fatalf("recommendation should name the env var; got %q", rec)
	}
}

func TestQueryDoctorRejectsRawPIT(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "pit-example-token")
	t.Setenv("TOPLINE_QUERY_BASE_URL", "http://127.0.0.1:1")

	var stdout bytes.Buffer
	if err := Execute([]string{"query", "doctor"}, &stdout, io.Discard); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := decodeDoctor(t, stdout.String())
	if got, _ := out["rawPitRejected"].(bool); !got {
		t.Fatalf("rawPitRejected should be true for pit-* tokens")
	}
	if got, _ := out["schemaReachable"].(bool); got {
		t.Fatalf("schemaReachable should be false when rejecting raw PIT")
	}
}

func TestQueryDoctorReportsTablePresence(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "signed-query-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/api/get-overview" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tables":[
			{"name":"contacts","row_count":10},
			{"name":"opportunities","row_count":20},
			{"name":"messages","row_count":30},
			{"name":"pipelines","row_count":4},
			{"name":"pipeline_stages","row_count":12},
			{"name":"call_events","row_count":5},
			{"name":"appointments","row_count":3},
			{"name":"conversations","row_count":7}
		]}`))
	}))
	defer server.Close()
	t.Setenv("TOPLINE_QUERY_BASE_URL", server.URL)

	var stdout bytes.Buffer
	if err := Execute([]string{"query", "doctor"}, &stdout, io.Discard); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := decodeDoctor(t, stdout.String())
	if got, _ := out["schemaReachable"].(bool); !got {
		t.Fatalf("schemaReachable should be true; got %v", out)
	}
	if n, _ := out["tableCount"].(float64); int(n) != 8 {
		t.Fatalf("tableCount = %v, want 8", n)
	}
	expected, _ := out["expectedTables"].(map[string]any)
	for _, table := range []string{"contacts", "opportunities", "messages", "pipeline_stages"} {
		if got, _ := expected[table].(bool); !got {
			t.Fatalf("expectedTables[%q] = %v, want true", table, expected[table])
		}
	}
	rec, _ := out["recommendation"].(string)
	if !strings.Contains(rec, "ready") {
		t.Fatalf("recommendation should report ready; got %q", rec)
	}
}

func TestQueryDoctorFlagsMissingTables(t *testing.T) {
	t.Setenv("TOPLINE_QUERY_TOKEN", "signed-query-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tables":[
			{"name":"contacts","row_count":10},
			{"name":"opportunities","row_count":20}
		]}`))
	}))
	defer server.Close()
	t.Setenv("TOPLINE_QUERY_BASE_URL", server.URL)

	var stdout bytes.Buffer
	if err := Execute([]string{"query", "doctor"}, &stdout, io.Discard); err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	out := decodeDoctor(t, stdout.String())
	missing, _ := out["missingTables"].([]any)
	if len(missing) == 0 {
		t.Fatalf("expected missingTables to include core analytics tables")
	}
	rec, _ := out["recommendation"].(string)
	if !strings.Contains(rec, "coverage") {
		t.Fatalf("recommendation should call out coverage gap; got %q", rec)
	}
}
