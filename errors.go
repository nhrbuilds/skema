package skema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Issue is a single validation problem at a JSON path.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError aggregates all issues from one parse pass.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Issues))
	for _, iss := range e.Issues {
		if iss.Path != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", iss.Path, iss.Message))
		} else {
			parts = append(parts, iss.Message)
		}
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Add appends an issue.
func (e *ValidationError) Add(path, code, msg string) {
	e.Issues = append(e.Issues, Issue{Path: path, Code: code, Message: msg})
}

func newValidationError() *ValidationError {
	return &ValidationError{Issues: make([]Issue, 0, 4)}
}

func joinPath(parent, segment string) string {
	if parent == "" {
		return segment
	}
	if segment == "" {
		return parent
	}
	if strings.HasPrefix(segment, "[") {
		return parent + segment
	}
	return parent + "." + segment
}

// IssueCollector lets refinement functions (SuperRefine) add issues at
// arbitrary paths relative to the refined value.
type IssueCollector struct {
	path string
	ve   *ValidationError
}

// Add appends an issue. The path is joined with the refined value's path,
// e.g. Add("startDate", CodeCustom, "...") reports at "range.startDate".
func (c *IssueCollector) Add(path, code, msg string) {
	c.ve.Add(joinPath(c.path, path), code, msg)
}

// MarshalJSON implements json.Marshaler for HTTP responses.
func (e *ValidationError) MarshalJSON() ([]byte, error) {
	type envelope struct {
		Error  string  `json:"error"`
		Issues []Issue `json:"issues"`
	}
	return json.Marshal(envelope{
		Error:  "validation_failed",
		Issues: e.Issues,
	})
}
