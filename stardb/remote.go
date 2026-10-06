package stardb

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
)

// RemoteError reports a hosted backend failure without copying response bodies,
// credentials, or request URLs into logs and error strings.
type RemoteError struct {
	Backend string
	Status  int
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("stardb: %s returned HTTP %d", e.Backend, e.Status)
}

// Temporary reports whether retrying later may succeed. StarDB does not retry
// writes automatically because applications may need stronger idempotency.
func (e *RemoteError) Temporary() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

func secureBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("stardb: invalid remote URL")
	}
	if parsed.Scheme != "https" {
		return nil, ErrInsecureURL
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("stardb: remote URL cannot contain credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

func safeHTTPClient(source *http.Client) *http.Client {
	copy := *source
	copy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copy
}

func remoteRequest(ctx context.Context, client *http.Client, method string, endpoint *url.URL, headers http.Header, body any, limit int64, destination any, backend string, accepted ...int) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("stardb: encode %s request: %w", backend, err)
		}
		if int64(len(encoded)) > limit {
			return nil, ErrTooLarge
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("stardb: create %s request", backend)
	}
	request.Header = headers.Clone()
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("stardb: %s request failed", backend)
	}
	ok := false
	for _, status := range accepted {
		if response.StatusCode == status {
			ok = true
			break
		}
	}
	if !ok {
		_ = drainAndClose(response.Body, limit)
		switch response.StatusCode {
		case http.StatusNotFound:
			return response, ErrNotFound
		case http.StatusConflict, http.StatusPreconditionFailed:
			return response, ErrConflict
		case http.StatusRequestEntityTooLarge:
			return response, ErrTooLarge
		default:
			return response, &RemoteError{Backend: backend, Status: response.StatusCode}
		}
	}
	if destination == nil {
		if err := drainAndClose(response.Body, limit); err != nil {
			return response, err
		}
		return response, nil
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, limit+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return response, fmt.Errorf("stardb: read %s response", backend)
	}
	if int64(len(encoded)) > limit {
		return response, ErrTooLarge
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return response, fmt.Errorf("stardb: decode %s response: %w", backend, err)
	}
	return response, nil
}

func drainAndClose(body io.ReadCloser, limit int64) error {
	defer body.Close()
	read, err := io.Copy(io.Discard, io.LimitReader(body, limit+1))
	if err != nil {
		return err
	}
	if read > limit {
		return ErrTooLarge
	}
	return nil
}
