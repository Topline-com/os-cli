package topline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultQueryBaseURL = "https://os-mcp.topline.com"

type QueryConfig struct {
	BaseURL string
	Token   string
}

type QueryClient struct {
	cfg        QueryConfig
	httpClient *http.Client
}

type QueryAPIError struct {
	StatusCode int
	Body       any
	Message    string
}

func (e *QueryAPIError) Error() string {
	return fmt.Sprintf("Topline query API error %d: %s", e.StatusCode, e.Message)
}

// QueryEnvStatus reports what the CLI sees in the environment for the SQL/query
// surface without erroring. Use this for `query doctor`; use LoadQueryConfig for
// commands that must hard-fail when the token is missing or unusable.
type QueryEnvStatus struct {
	BaseURL      string
	Token        string
	TokenPresent bool
	RawPITToken  bool
	SourceEnvVar string
}

func InspectQueryEnv() QueryEnvStatus {
	status := QueryEnvStatus{
		BaseURL: strings.TrimSpace(os.Getenv("TOPLINE_QUERY_BASE_URL")),
	}
	if status.BaseURL == "" {
		status.BaseURL = DefaultQueryBaseURL
	}
	for _, key := range []string{"TOPLINE_QUERY_TOKEN", "TOPLINE_MCP_ACCESS_TOKEN", "TOPLINE_MCP_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			status.Token = v
			status.TokenPresent = true
			status.SourceEnvVar = key
			break
		}
	}
	if status.TokenPresent && strings.HasPrefix(status.Token, "pit-") {
		status.RawPITToken = true
	}
	return status
}

func LoadQueryConfig() (QueryConfig, error) {
	status := InspectQueryEnv()
	cfg := QueryConfig{BaseURL: status.BaseURL, Token: status.Token}
	if !status.TokenPresent {
		return cfg, errors.New("TOPLINE_QUERY_TOKEN is required for SQL/query commands; generate a connection-bound token at https://os-mcp.topline.com/connect or set TOPLINE_MCP_ACCESS_TOKEN")
	}
	if status.RawPITToken {
		return cfg, errors.New("TOPLINE_QUERY_TOKEN must be a connection-bound MCP/query token, not a raw PIT; generate one at https://os-mcp.topline.com/connect")
	}
	return cfg, nil
}

func NewQueryClient(cfg QueryConfig) *QueryClient {
	return &QueryClient{cfg: cfg, httpClient: &http.Client{Timeout: 60 * time.Second}}
}

func (c *QueryClient) Get(ctx context.Context, path string, values url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, values, nil, out)
}

func (c *QueryClient) PostJSON(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out)
}

func (c *QueryClient) do(ctx context.Context, method, path string, values url.Values, body any, out any) error {
	endpoint, err := c.url(path, values)
	if err != nil {
		return err
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var parsed any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			parsed = string(raw)
		}
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if out == nil || len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, out)
	}
	return shapeQueryAPIError(res.StatusCode, parsed)
}

func (c *QueryClient) url(path string, values url.Values) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	if len(values) > 0 {
		q := u.Query()
		for key, vals := range values {
			for _, v := range vals {
				if v != "" {
					q.Add(key, v)
				}
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func shapeQueryAPIError(status int, body any) error {
	msg := fmt.Sprintf("HTTP %d", status)
	if m, ok := body.(map[string]any); ok {
		if s, ok := m["error"].(string); ok && s != "" {
			msg = s
		} else if s, ok := m["message"].(string); ok && s != "" {
			msg = s
		}
	} else if s, ok := body.(string); ok && strings.TrimSpace(s) != "" {
		msg = strings.TrimSpace(s)
	}
	msg = strings.ReplaceAll(msg, "Bearer ", "Bearer [REDACTED] ")
	if status == http.StatusUnauthorized {
		msg = "Authentication failed for Topline query API. Use a connection-bound TOPLINE_QUERY_TOKEN, not TOPLINE_PIT."
	} else if status == http.StatusForbidden {
		msg = "Forbidden by Topline query API. The token is missing access to this location or SQL surface."
	}
	return &QueryAPIError{StatusCode: status, Body: body, Message: msg}
}
