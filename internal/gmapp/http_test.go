package gmapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMux(token string) http.Handler {
	app := New(":0", "nats://127.0.0.1:4222", "mmo", "10001", token)
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	return mux
}

func TestHandleHealthOK(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var rsp healthRsp
	if err := json.NewDecoder(rec.Body).Decode(&rsp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rsp.Code != 0 || rsp.Message != "ok" {
		t.Fatalf("unexpected health rsp: %+v", rsp)
	}
}

func TestHandleConfigReloadUnauthorized(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/config/reload", strings.NewReader(`{"tableName":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
}

func TestHandleConfigReloadWrongToken(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/config/reload", strings.NewReader(`{"tableName":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "wrong")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
}

func TestHandleConfigReloadEmptyConfiguredToken(t *testing.T) {
	mux := newTestMux("")
	req := httptest.NewRequest(http.MethodPost, "/gm/config/reload", strings.NewReader(`{"tableName":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "anything")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
}

func TestHandleConfigReloadBearerAndBadJSON(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/config/reload", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "not-json") {
		t.Fatalf("response leaked parse error: %s", rec.Body.String())
	}
}

func TestHandleConfigReloadMethodNotAllowed(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/config/reload", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", rec.Code)
	}
}

func TestCompareGMToken(t *testing.T) {
	if !compareGMToken("abc", "abc") {
		t.Fatal("equal tokens should match")
	}
	if compareGMToken("abc", "abd") {
		t.Fatal("different tokens should not match")
	}
	if compareGMToken("abc", "") {
		t.Fatal("empty provided should not match")
	}
}
