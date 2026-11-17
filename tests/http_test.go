package skema_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	sk "github.com/nhrbuilds/skema"
)

type httpName struct {
	Name string `json:"name"`
}

func TestDecodeHTTPValidation422(t *testing.T) {
	schema := sk.Struct(func(s *sk.ObjectBuilder[httpName]) {
		sk.Field(s, &s.Target.Name, sk.String().Min(2))
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":"a"}`))
	req.Header.Set("Content-Type", "application/json")

	_, err := sk.DecodeHTTP(req, schema)
	if err == nil {
		t.Fatal("expected error")
	}

	rr := httptest.NewRecorder()
	sk.WriteValidationError(rr, err)
	if rr.Code != 422 {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}

func TestDecodeHTTPTrailingJSON(t *testing.T) {
	schema := sk.String()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`"ok" null`))
	req.Header.Set("Content-Type", "application/json")
	_, err := sk.DecodeHTTP(req, schema)
	if err == nil {
		t.Fatal("expected trailing json error")
	}
}
