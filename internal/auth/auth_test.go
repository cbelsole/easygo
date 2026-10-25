package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"easygo/internal/store"
)

func newAuth(t *testing.T, cfg Config) (*Auth, *http.ServeMux) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if cfg.Secret == nil {
		cfg.Secret = NewSecret()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	a := New(cfg, st)
	mux := http.NewServeMux()
	a.Register(mux)
	return a, mux
}

func post(mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDevLoginSetsAVerifiableSession(t *testing.T) {
	a, mux := newAuth(t, Config{DevLogin: true})
	rec := post(mux, "/api/auth/dev-login", `{"name":"Ann Lee"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var u store.User
	json.Unmarshal(rec.Body.Bytes(), &u)
	if u.ID != "dev:ann-lee" || u.Name != "Ann Lee" {
		t.Errorf("user %+v", u)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
		if !c.HttpOnly {
			t.Error("session cookie must be HttpOnly")
		}
	}
	if got := a.UserID(req); got != "dev:ann-lee" {
		t.Errorf("UserID = %q", got)
	}
	if cu, ok := a.User(req); !ok || cu.Name != "Ann Lee" {
		t.Errorf("User: %+v %v", cu, ok)
	}
}

func TestDevLoginDisabledByDefault(t *testing.T) {
	_, mux := newAuth(t, Config{})
	if rec := post(mux, "/api/auth/dev-login", `{"name":"x"}`); rec.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rec.Code)
	}
}

func TestDevLoginRejectsBadNames(t *testing.T) {
	_, mux := newAuth(t, Config{DevLogin: true})
	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{"name":"!!!"}`, `{"name":"` + strings.Repeat("a", 41) + `"}`, `not json`} {
		if rec := post(mux, "/api/auth/dev-login", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%q gave %d", body, rec.Code)
		}
	}
}

func TestTamperedAndExpiredCookiesAreRejected(t *testing.T) {
	a, _ := newAuth(t, Config{})
	good := a.sign("dev:ann", time.Now().Add(time.Hour).Unix())
	if id, ok := a.verify(good); !ok || id != "dev:ann" {
		t.Fatalf("valid cookie rejected: %q %v", id, ok)
	}
	parts := strings.Split(good, ".")
	// Swap in another user's ID, keeping the old signature.
	forged := "ZGV2OmJvYg." + parts[1] + "." + parts[2]
	if _, ok := a.verify(forged); ok {
		t.Error("forged user accepted")
	}
	if _, ok := a.verify(a.sign("dev:ann", time.Now().Add(-time.Minute).Unix())); ok {
		t.Error("expired cookie accepted")
	}
	other, _ := newAuth(t, Config{})
	if _, ok := other.verify(good); ok {
		t.Error("cookie signed with another secret accepted")
	}
	for _, junk := range []string{"", "a.b", "a.b.c.d", "...."} {
		if _, ok := a.verify(junk); ok {
			t.Errorf("junk %q accepted", junk)
		}
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	_, mux := newAuth(t, Config{})
	rec := post(mux, "/api/auth/logout", "")
	if rec.Code != 204 {
		t.Fatalf("status %d", rec.Code)
	}
	cs := rec.Result().Cookies()
	if len(cs) == 0 || cs[0].MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", cs)
	}
}

func TestGoogleDisabledWithoutCredentials(t *testing.T) {
	a, mux := newAuth(t, Config{})
	if a.GoogleEnabled() {
		t.Fatal("google should be off")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/google/login", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d", rec.Code)
	}
}

func TestGoogleLoginRedirectsWithState(t *testing.T) {
	_, mux := newAuth(t, Config{GoogleID: "cid", GoogleSecret: "sec", BaseURL: "http://localhost:8080"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/auth/google/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "accounts.google.com" || loc.Query().Get("client_id") != "cid" {
		t.Errorf("redirect %s", loc)
	}
	if loc.Query().Get("redirect_uri") != "http://localhost:8080/auth/google/callback" {
		t.Errorf("redirect_uri %q", loc.Query().Get("redirect_uri"))
	}
	state := loc.Query().Get("state")
	var cookieState string
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			cookieState = c.Value
		}
	}
	if state == "" || state != cookieState {
		t.Errorf("state %q vs cookie %q", state, cookieState)
	}
}

func TestGoogleCallbackRejectsBadState(t *testing.T) {
	_, mux := newAuth(t, Config{GoogleID: "cid", GoogleSecret: "sec"})
	for _, tc := range []struct{ name, query, cookie string }{
		{"no cookie", "state=abc&code=x", ""},
		{"mismatch", "state=abc&code=x", "def"},
		{"empty both", "state=&code=x", ""},
	} {
		req := httptest.NewRequest("GET", "/auth/google/callback?"+tc.query, nil)
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: stateCookie, Value: tc.cookie})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", tc.name, rec.Code)
		}
	}
}
