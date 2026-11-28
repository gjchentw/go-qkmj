package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRqliteTimeout  = 5 * time.Second
	maxRqliteRedirectHops = 3
	maxRqliteErrorBytes   = 4096
)

type rqliteExecutor struct {
	endpoints   []string
	consistency ReadConsistency
	timeout     time.Duration
	client      *http.Client
}

var _ executor = (*rqliteExecutor)(nil)

func newRqliteExecutor(
	endpoints []string,
	consistency ReadConsistency,
	timeout time.Duration,
	httpClient *http.Client,
) (*rqliteExecutor, error) {
	cleaned, err := normalizeRqliteEndpoints(endpoints)
	if err != nil {
		return nil, err
	}
	if !consistency.Valid() {
		return nil, fmt.Errorf("invalid rqlite read consistency %v: %w", consistency, ErrInvalidConfig)
	}
	if timeout <= 0 {
		timeout = defaultRqliteTimeout
	}

	client := cloneHTTPClientNoAutoRedirect(httpClient, timeout)
	return &rqliteExecutor{
		endpoints:   cleaned,
		consistency: consistency,
		timeout:     timeout,
		client:      client,
	}, nil
}

func normalizeRqliteEndpoints(endpoints []string) ([]string, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("rqlite endpoints must not be empty: %w", ErrInvalidConfig)
	}
	cleaned := make([]string, 0, len(endpoints))
	for _, raw := range endpoints {
		trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
		u, err := url.Parse(trimmed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("invalid rqlite endpoint %q: %w", raw, ErrInvalidConfig)
		}
		cleaned = append(cleaned, trimmed)
	}
	return cleaned, nil
}

func cloneHTTPClientNoAutoRedirect(base *http.Client, timeout time.Duration) *http.Client {
	c := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if base != nil {
		c.Transport = base.Transport
	}
	return c
}

type rqliteResponse struct {
	Results []rqliteResult `json:"results"`
}

type rqliteResult struct {
	Columns      []string `json:"columns"`
	Values       [][]any  `json:"values"`
	LastInsertID int64    `json:"last_insert_id"`
	RowsAffected int64    `json:"rows_affected"`
	Error        string   `json:"error"`
}

func encodeRqliteBatch(stmts []stmt) ([]byte, error) {
	payload := make([][]any, 0, len(stmts))
	for _, s := range stmts {
		entry := make([]any, 0, 1+len(s.args))
		entry = append(entry, s.query)
		entry = append(entry, s.args...)
		payload = append(payload, entry)
	}
	return json.Marshal(payload)
}

func (e *rqliteExecutor) Query(ctx context.Context, s stmt) ([][]any, error) {
	body, err := encodeRqliteBatch([]stmt{s})
	if err != nil {
		return nil, fmt.Errorf("encode rqlite query: %w", err)
	}
	path := "/db/query?level=" + e.consistency.String()
	parsed, err := e.postWithFailover(ctx, path, body)
	if err != nil {
		return nil, err
	}
	if len(parsed.Results) == 0 {
		return nil, fmt.Errorf("rqlite query returned empty results: %w", ErrBackendFailure)
	}
	if parsed.Results[0].Error != "" {
		return nil, fmt.Errorf("rqlite query error: %s: %w", parsed.Results[0].Error, ErrBackendFailure)
	}
	return parsed.Results[0].Values, nil
}

func (e *rqliteExecutor) ExecBatch(ctx context.Context, stmts []stmt) ([]execResult, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	body, err := encodeRqliteBatch(stmts)
	if err != nil {
		return nil, fmt.Errorf("encode rqlite execute batch: %w", err)
	}
	parsed, err := e.postWithFailover(ctx, "/db/execute?transaction=true", body)
	if err != nil {
		return nil, err
	}
	if len(parsed.Results) == 0 {
		return nil, fmt.Errorf("rqlite execute returned empty results: %w", ErrBackendFailure)
	}

	out := make([]execResult, 0, len(parsed.Results))
	for i, res := range parsed.Results {
		if res.Error != "" {
			return nil, fmt.Errorf("rqlite exec stmt[%d]: %s: %w", i, res.Error, ErrBackendFailure)
		}
		out = append(out, execResult{
			lastInsertID: res.LastInsertID,
			rowsAffected: res.RowsAffected,
		})
	}
	return out, nil
}

func (e *rqliteExecutor) postWithFailover(ctx context.Context, reqPath string, body []byte) (rqliteResponse, error) {
	var errs []error
	for _, base := range e.endpoints {
		if err := ctx.Err(); err != nil {
			return rqliteResponse{}, err
		}
		parsed, err := e.postEndpoint(ctx, base+reqPath, body)
		if err == nil {
			return parsed, nil
		}
		errs = append(errs, fmt.Errorf("endpoint %s: %w", base, err))
	}
	return rqliteResponse{}, fmt.Errorf("all rqlite endpoints failed: %w", errors.Join(errs...))
}

func (e *rqliteExecutor) postEndpoint(ctx context.Context, targetURL string, body []byte) (rqliteResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	currentURL := targetURL
	for range maxRqliteRedirectHops + 1 {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, currentURL, bytes.NewReader(body))
		if err != nil {
			return rqliteResponse{}, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := e.client.Do(req)
		if err != nil {
			return rqliteResponse{}, fmt.Errorf("http post: %w", err)
		}

		nextURL, redirected, err := checkRqliteRedirect(resp, currentURL)
		if err != nil {
			_ = resp.Body.Close()
			return rqliteResponse{}, err
		}
		if redirected {
			if _, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxRqliteErrorBytes)); copyErr != nil {
				_ = resp.Body.Close()
				return rqliteResponse{}, fmt.Errorf("drain redirect body: %w", copyErr)
			}
			_ = resp.Body.Close()
			currentURL = nextURL
			continue
		}
		return decodeRqliteResponse(resp)
	}
	return rqliteResponse{}, fmt.Errorf("exceeded %d redirects: %w", maxRqliteRedirectHops, ErrBackendFailure)
}

func checkRqliteRedirect(resp *http.Response, currentURL string) (nextURL string, redirected bool, err error) {
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", false, fmt.Errorf("redirect without Location header: %w", ErrBackendFailure)
		}
		base, parseErr := url.Parse(currentURL)
		if parseErr != nil {
			return "", false, parseErr
		}
		ref, parseErr := url.Parse(loc)
		if parseErr != nil {
			return "", false, parseErr
		}
		return base.ResolveReference(ref).String(), true, nil
	default:
		return "", false, nil
	}
}

func decodeRqliteResponse(resp *http.Response) (rqliteResponse, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxRqliteErrorBytes))
		if readErr != nil {
			return rqliteResponse{}, fmt.Errorf("http %d (read body failed: %w): %w", resp.StatusCode, readErr, ErrBackendFailure)
		}
		return rqliteResponse{}, fmt.Errorf("http %d: %s: %w", resp.StatusCode, strings.TrimSpace(string(snippet)), ErrBackendFailure)
	}
	var parsed rqliteResponse
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		return rqliteResponse{}, fmt.Errorf("decode rqlite json: %w", err)
	}
	return parsed, nil
}

func (e *rqliteExecutor) Close() error {
	e.client.CloseIdleConnections()
	return nil
}
