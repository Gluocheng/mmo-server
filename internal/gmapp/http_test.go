package gmapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/mmo-server/gameconfig/pkg/schema"
	"github.com/example/mmo-server/internal/code"
	"github.com/example/mmo-server/internal/persistence"
)

func testMux(token string) http.Handler {
	a := New(":0", "nats://127.0.0.1:4222", "mmo", "10001", token)
	a.remoteSubject = "cherry-mmo.remote.game.10001"
	a.targetPath = "10001.gm.config"
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	return mux
}

func TestHealthNoAuth(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp healthRsp
	if err := json.Unmarshal(rec.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Code != 0 {
		t.Fatalf("code=%d", rsp.Code)
	}
}

func TestBusinessRequiresToken(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/bag/grant", strings.NewReader(`{"playerId":1,"itemId":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp errorRsp
	if err := json.Unmarshal(rec.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Code != code.GmUnauthorized {
		t.Fatalf("code=%d", rsp.Code)
	}
}

func TestWrongTokenRejected(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/account?uid=1", nil)
	req.Header.Set("X-GM-Token", "nope")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestBearerTokenAcceptedThenNATSDown(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/account?uid=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEmptyTokenRejectsBusiness(t *testing.T) {
	mux := testMux("")
	req := httptest.NewRequest(http.MethodGet, "/gm/account?uid=1", nil)
	req.Header.Set("X-GM-Token", "anything")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestAccountXORParams(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/account", nil)
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp errorRsp
	_ = json.Unmarshal(rec.Body.Bytes(), &rsp)
	if rsp.Code != code.GmBadRequest {
		t.Fatalf("code=%d", rsp.Code)
	}
}

func TestDeductXORParams(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/bag/deduct", strings.NewReader(`{"playerId":1,"itemId":1,"slot":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBanNegativeDuration(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/account/ban", strings.NewReader(`{"uid":1,"durationSeconds":-1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMuteXORParams(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/account/mute", strings.NewReader(`{"uid":1,"nickname":"a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMaintenanceGetAuthorized(t *testing.T) {
	persistence.UseMemoryDBForTest(t)
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/maintenance", nil)
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp["code"] != float64(0) {
		t.Fatalf("code=%v body=%s", rsp["code"], rec.Body.String())
	}
	if _, ok := rsp["enabled"]; !ok {
		t.Fatalf("missing enabled: %s", rec.Body.String())
	}
}

func TestNoticeEmptyText(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/notice", strings.NewReader(`{"sceneId":0,"text":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTimeGetAuthorized(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/time", nil)
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp["code"] != float64(0) {
		t.Fatalf("code=%v body=%s", rsp["code"], rec.Body.String())
	}
	if _, ok := rsp["biasSeconds"]; !ok {
		t.Fatalf("missing biasSeconds: %s", rec.Body.String())
	}
}

func TestP1RoutesRequireAuth(t *testing.T) {
	mux := testMux("secret")
	paths := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/gm/config/catalog", ""},
		{http.MethodPost, "/gm/bag/deduct", `{"playerId":1,"itemId":1}`},
		{http.MethodPost, "/gm/account/ban", `{"uid":1}`},
		{http.MethodPost, "/gm/account/mute", `{"uid":1}`},
		{http.MethodGet, "/gm/world/online", ""},
		{http.MethodPost, "/gm/notice", `{"text":"hi"}`},
		{http.MethodGet, "/gm/time", ""},
		{http.MethodGet, "/gm/maintenance", ""},
	}
	for _, tc := range paths {
		var req *http.Request
		if tc.body != "" {
			req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestKickXORParams(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodPost, "/gm/player/kick", strings.NewReader(`{"uid":1,"playerId":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSPARootReturnsHTML(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%s", ct)
	}
	if !strings.Contains(rec.Body.String(), "html") {
		t.Fatalf("expected html, got %s", rec.Body.String())
	}
}

func TestSPAFallbackDoesNotCaptureHealth(t *testing.T) {
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/player", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback status=%d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("fallback content-type=%s", rec.Header().Get("Content-Type"))
	}

	hreq := httptest.NewRequest(http.MethodGet, "/gm/health", nil)
	hrec := httptest.NewRecorder()
	mux.ServeHTTP(hrec, hreq)
	if hrec.Code != http.StatusOK {
		t.Fatalf("health status=%d", hrec.Code)
	}
	if !strings.Contains(hrec.Header().Get("Content-Type"), "json") {
		t.Fatalf("health should stay json, got %s body=%s", hrec.Header().Get("Content-Type"), hrec.Body.String())
	}
}

func TestLoginSessionAndAdminGate(t *testing.T) {
	persistence.UseMemoryDBForTest(t)
	if _, err := persistence.CreateGMUser("admin", "admin123", "Admin", persistence.GMRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := persistence.CreateGMUser("op1", "oppass12", "Op", persistence.GMRoleOperator); err != nil {
		t.Fatal(err)
	}
	mux := testMux("secret")

	bad := httptest.NewRequest(http.MethodPost, "/gm/auth/login", strings.NewReader(`{"username":"admin","password":"nope123"}`))
	bad.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d body=%s", badRec.Code, badRec.Body.String())
	}

	login := httptest.NewRequest(http.MethodPost, "/gm/auth/login", strings.NewReader(`{"username":"admin","password":"admin123"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == gmSessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("missing session cookie")
	}

	me := httptest.NewRequest(http.MethodGet, "/gm/auth/me", nil)
	me.AddCookie(cookie)
	meRec := httptest.NewRecorder()
	mux.ServeHTTP(meRec, me)
	if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), `"admin"`) {
		t.Fatalf("me status=%d body=%s", meRec.Code, meRec.Body.String())
	}

	opLogin := httptest.NewRequest(http.MethodPost, "/gm/auth/login", strings.NewReader(`{"username":"op1","password":"oppass12"}`))
	opLogin.Header.Set("Content-Type", "application/json")
	opRec := httptest.NewRecorder()
	mux.ServeHTTP(opRec, opLogin)
	var opCookie *http.Cookie
	for _, c := range opRec.Result().Cookies() {
		if c.Name == gmSessionCookie {
			opCookie = c
		}
	}
	if opCookie == nil {
		t.Fatal("op cookie")
	}
	users := httptest.NewRequest(http.MethodGet, "/gm/users", nil)
	users.AddCookie(opCookie)
	usersRec := httptest.NewRecorder()
	mux.ServeHTTP(usersRec, users)
	if usersRec.Code != http.StatusForbidden {
		t.Fatalf("op users status=%d body=%s", usersRec.Code, usersRec.Body.String())
	}
	var ferr errorRsp
	_ = json.Unmarshal(usersRec.Body.Bytes(), &ferr)
	if ferr.Code != code.GmForbidden {
		t.Fatalf("forbidden code=%d", ferr.Code)
	}

	logs := httptest.NewRequest(http.MethodGet, "/gm/ops/logs?page=1&pageSize=10", nil)
	logs.AddCookie(cookie)
	logsRec := httptest.NewRecorder()
	mux.ServeHTTP(logsRec, logs)
	if logsRec.Code != http.StatusOK {
		t.Fatalf("logs status=%d body=%s", logsRec.Code, logsRec.Body.String())
	}
}

func TestConfigCatalogListsNames(t *testing.T) {
	gdb := persistence.UseMemoryDBForTest(t)
	if err := gdb.Create(&schema.CfgBagType{ID: 2, Name: "消耗品", SlotCount: 32}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Create(&schema.CfgItem{
		ID: 1001, Name: "小型生命药水", Type: "consumable", MaxStack: 99, Stackable: true, Discardable: true, BindType: "none", BagType: 2,
	}).Error; err != nil {
		t.Fatal(err)
	}
	mux := testMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/gm/config/catalog", nil)
	req.Header.Set("X-GM-Token", "secret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rsp catalogRsp
	if err := json.Unmarshal(rec.Body.Bytes(), &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Code != 0 || len(rsp.BagTypes) != 1 || rsp.BagTypes[0].Name != "消耗品" {
		t.Fatalf("bagTypes=%+v", rsp.BagTypes)
	}
	if len(rsp.Items) != 1 || rsp.Items[0].Name != "小型生命药水" {
		t.Fatalf("items=%+v", rsp.Items)
	}
}
