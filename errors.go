package starlings

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrClosed is returned by operations on a Client whose gateway connection has
// been shut down.
var ErrClosed = errors.New("starlings: client closed")

// APIError is returned when Discord answers a REST call with a non-2xx status.
// Discord's own error code (distinct from the HTTP status) is in Code, and
// per-field validation failures land in Errors.
type APIError struct {
	Status  int    // HTTP status code
	Code    int    // Discord JSON error code, 0 if absent
	Message string // Discord's human-readable message
	Errors  []FieldError
	Body    []byte // raw response body, for anything not modelled above
}

// FieldError is a single validation failure against one field of the request.
type FieldError struct {
	Path    string // dotted path, e.g. "embeds.0.description"
	Code    string
	Message string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	s := fmt.Sprintf("starlings: discord returned %d", e.Status)
	if e.Code != 0 {
		s += fmt.Sprintf(" (code %d)", e.Code)
	}
	s += ": " + sanitizeUntrustedText(msg)
	for _, fe := range e.Errors {
		s += fmt.Sprintf("\n  %s: %s", sanitizeUntrustedText(fe.Path), sanitizeUntrustedText(fe.Message))
	}
	if len(e.Errors) == 0 && len(e.Body) > 0 {
		s += fmt.Sprintf("\n  body: %s", sanitizeUntrustedText(string(e.Body)))
	}
	return s
}

// HTTPStatus reports the HTTP status of err if it is an APIError, and 0 otherwise.
// Useful with errors.As-free call sites:
//
//	if starlings.HTTPStatus(err) == http.StatusNotFound { ... }
func HTTPStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// IsNotFound reports whether err is a 404 from Discord.
func IsNotFound(err error) bool { return HTTPStatus(err) == http.StatusNotFound }

// IsUnauthorized reports whether err is a 401, which almost always means the
// bot token is wrong or has been reset.
func IsUnauthorized(err error) bool { return HTTPStatus(err) == http.StatusUnauthorized }

// IsForbidden reports whether err is a 403, which means the token is valid but
// the bot lacks the permission for this action.
func IsForbidden(err error) bool { return HTTPStatus(err) == http.StatusForbidden }

// errNotReady is returned by calls that need information the gateway only
// supplies once READY has arrived, such as the application ID.
var errNotReady = errors.New(
	"starlings: not connected yet - call this from a Ready handler, or after WaitReady")
