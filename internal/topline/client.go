package topline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Request struct {
	Method string
	Path   string
	Query  map[string]string
	Body   map[string]any
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

type APIError struct {
	StatusCode int
	Body       any
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Topline API error %d: %s", e.StatusCode, e.Message)
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, httpClient: &http.Client{Timeout: 60 * time.Second}}
}

func (c *Client) Do(ctx context.Context, req Request, out any) error {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}
	endpoint, err := c.url(req.Path, req.Query)
	if err != nil {
		return err
	}
	var payload []byte
	if req.Body != nil && method != http.MethodGet {
		payload, err = json.Marshal(req.Body)
		if err != nil {
			return err
		}
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		hreq, err := http.NewRequestWithContext(ctx, method, endpoint, body)
		if err != nil {
			return err
		}
		hreq.Header.Set("Authorization", "Bearer "+c.cfg.PIT)
		hreq.Header.Set("Version", APIVersion)
		hreq.Header.Set("Accept", "application/json")
		if payload != nil {
			hreq.Header.Set("Content-Type", "application/json")
		}
		res, err := c.httpClient.Do(hreq)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var parsed any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &parsed); err != nil {
				parsed = string(raw)
			}
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if out == nil {
				return nil
			}
			if len(raw) == 0 {
				return nil
			}
			return json.Unmarshal(raw, out)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := 500 * time.Millisecond * time.Duration(1<<(attempt-1))
			if retryAfter := res.Header.Get("Retry-After"); retryAfter != "" {
				if d, err := time.ParseDuration(retryAfter + "s"); err == nil {
					delay = d
				}
			}
			time.Sleep(delay)
			continue
		}
		return shapeAPIError(res.StatusCode, parsed)
	}
	return lastErr
}

func (c *Client) url(path string, query map[string]string) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range query {
		if v == "" {
			continue
		}
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func shapeAPIError(status int, body any) error {
	msg := fmt.Sprintf("HTTP %d", status)
	if m, ok := body.(map[string]any); ok {
		if s, ok := m["message"].(string); ok && s != "" {
			msg = s
		}
	}
	if status == http.StatusUnauthorized {
		msg = "Authentication failed. TOPLINE_PIT is invalid or expired."
	} else if status == http.StatusForbidden {
		msg = "Forbidden. The Private Integration is missing a required scope."
	} else if status == http.StatusTooManyRequests {
		msg = "Rate limited by Topline OS. Try again shortly."
	}
	return &APIError{StatusCode: status, Body: body, Message: msg}
}
