package skema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPError is a non-validation HTTP failure.
type HTTPError struct {
	Status  int
	Code    string
	Message string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// HTTPOption configures DecodeHTTP.
type HTTPOption func(*httpDecodeConfig)

type httpDecodeConfig struct {
	maxBody int64
}

// WithMaxBodyBytes limits request body size.
func WithMaxBodyBytes(n int64) HTTPOption {
	return func(c *httpDecodeConfig) { c.maxBody = n }
}

type nopResponseWriter struct{}

func (nopResponseWriter) Header() http.Header       { return make(http.Header) }
func (nopResponseWriter) Write([]byte) (int, error) { return len([]byte{}), nil }
func (nopResponseWriter) WriteHeader(int)           {}

// DecodeHTTP decodes and validates a JSON request body.
func DecodeHTTP[T any](r *http.Request, schema Validator[T], opts ...HTTPOption) (T, error) {
	var zero T
	cfg := httpDecodeConfig{maxBody: 1 << 20}
	for _, opt := range opts {
		opt(&cfg)
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" {
		media := strings.Split(ct, ";")[0]
		if !strings.EqualFold(strings.TrimSpace(media), "application/json") {
			return zero, &HTTPError{Status: 415, Code: "unsupported_media_type", Message: "Content-Type must be application/json"}
		}
	}

	body := http.MaxBytesReader(nopResponseWriter{}, r.Body, cfg.maxBody)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return zero, &HTTPError{Status: 413, Code: "payload_too_large", Message: "request body too large"}
		}
		return zero, &HTTPError{Status: 400, Code: "invalid_json", Message: err.Error()}
	}
	if len(data) == 0 {
		return zero, &HTTPError{Status: 400, Code: "invalid_json", Message: "empty request body"}
	}

	out, err := schema.ParseBytes(data)
	if err != nil {
		return zero, err
	}
	return out, nil
}

// WriteValidationError writes a JSON error response aligned with ValidationError.
// Errors wrapping *HTTPError or *ValidationError are matched via errors.As.
func WriteValidationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var he *HTTPError
	if errors.As(err, &he) {
		writeHTTPError(w, he.Status, he.Code, he.Message, nil)
		return
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		writeHTTPError(w, http.StatusUnprocessableEntity, "validation_failed", ve.Error(), ve.Issues)
		return
	}
	writeHTTPError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
}

func writeHTTPError(w http.ResponseWriter, status int, code, message string, issues []Issue) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{
		"error": code,
	}
	if message != "" {
		payload["message"] = message
	}
	if issues != nil {
		payload["issues"] = issues
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// DrainBody consumes and closes a body (helper for tests).
func DrainBody(r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}
