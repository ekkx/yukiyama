package yukiyama

import (
	"encoding/json"
	"errors"
	"fmt"
)

// APIError wraps a wire-level failure response from the yukiyama API.
//
// SDK invariant: failure responses follow the envelope
//
//	{ "status": false | "0", "error": "<message>", "error_code": <int>? }
//
// The server frequently returns HTTP 200 even on logical failure, so error
// detection is body-based (see parseAPIErrorFromBody).
type APIError struct {
	StatusCode int    // HTTP status code (often 200 even when Status=false)
	Status     bool   // wire "status" normalized to bool (false = failure)
	Message    string // wire "error" field
	Code       int32  // wire "error_code" (0 = absent/unset)
	Body       []byte // raw response body for debugging
}

// Error implements error.
func (e *APIError) Error() string {
	if e == nil {
		return "yukiyama: <nil APIError>"
	}
	if e.Code != 0 {
		return fmt.Sprintf("yukiyama: API error (http=%d, code=%d): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("yukiyama: API error (http=%d): %s", e.StatusCode, e.Message)
}

// IsAuthExpired reports whether this error is the well-known
// "ログインセッションが終了しました" (error_code = 103). The authTransport uses
// this to decide whether to re-login and retry once.
func (e *APIError) IsAuthExpired() bool {
	return e != nil && e.Code == 103
}

// CodeOf extracts the wire error_code from an error chain. Returns 0 when err
// is not (and does not wrap) an *APIError.
func CodeOf(err error) int32 {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr.Code
	}
	return 0
}

// IsAuthExpired walks the err chain and reports whether any wrapped *APIError
// has error_code 103. Convenience for callers handling expired-session paths.
func IsAuthExpired(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsAuthExpired()
	}
	return false
}

// commonEnvelope is a minimal decode target for the universal failure shape.
// The yukiyama wire mixes types: status is sometimes `false`/`true` (bool),
// sometimes `"0"`/`"1"` (string), depending on endpoint. Decode as `any` then
// normalize.
type commonEnvelope struct {
	Status    any    `json:"status"`
	Error     string `json:"error"`
	ErrorCode *int32 `json:"error_code,omitempty"`
}

// parseAPIErrorFromBody returns a non-nil *APIError when body is a recognizable
// failure envelope ({status:false} or {status:"0"} with non-empty error). Returns
// nil when the body indicates success or is not a JSON envelope at all.
func parseAPIErrorFromBody(statusCode int, body []byte) *APIError {
	if len(body) == 0 {
		return nil
	}
	var env commonEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		// Not a JSON envelope (could be binary, HTML 502, etc). Caller can
		// still use the raw HTTP status path; we do not synthesize an
		// APIError from arbitrary bodies.
		return nil
	}
	statusBool, ok := normalizeStatus(env.Status)
	if !ok {
		// No `status` field present — not our envelope; do not flag as error.
		return nil
	}
	if statusBool {
		return nil
	}
	code := int32(0)
	if env.ErrorCode != nil {
		code = *env.ErrorCode
	}
	return &APIError{
		StatusCode: statusCode,
		Status:     false,
		Message:    env.Error,
		Code:       code,
		Body:       body,
	}
}

// normalizeStatus accepts bool or string ("0"/"1"/"true"/"false") and returns
// the bool value plus ok=true when we recognized one of those forms.
func normalizeStatus(v any) (bool, bool) {
	switch s := v.(type) {
	case bool:
		return s, true
	case string:
		switch s {
		case "1", "true", "True":
			return true, true
		case "0", "false", "False":
			return false, true
		}
	}
	return false, false
}
