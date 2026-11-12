package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"web-panel/internal/service"
	"web-panel/internal/store"
)

func TestAccessKeySessionProtectsAllAPIAndLogout(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.New(s, runtimeStub{})
	if err != nil {
		t.Fatal(err)
	}
	key := "webcp-unit-test-access-key"
	handler := New(m, fstest.MapFS{"index.html": {Data: []byte("WebCP login")}}, key)
	send := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/overview", "/api/version", "/api/config", "/api/backups/file/download", "/api/panel/websites/ssl/search"} {
		if w := send("GET", path, "", nil); w.Code != 401 {
			t.Fatal("unguarded API", path, w.Code)
		}
	}
	if w := send("GET", "/", "", nil); w.Code != 200 {
		t.Fatal("login page inaccessible")
	}
	if w := send("POST", "/api/auth/login", `{"key":"wrong"}`, nil); w.Code != 401 {
		t.Fatal("wrong key accepted")
	}
	body, _ := json.Marshal(map[string]string{"key": key})
	w := send("POST", "/api/auth/login", string(body), nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal("login failed", w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || strings.Contains(cookie.Value, key) {
		t.Fatal("unsafe session cookie")
	}
	if w = send("GET", "/api/overview", "", cookie); w.Code != 200 {
		t.Fatal("session rejected", w.Body.String())
	}
	send("POST", "/api/auth/logout", `{}`, cookie)
	if w = send("GET", "/api/overview", "", cookie); w.Code != 401 {
		t.Fatal("logout did not revoke session")
	}
	r := httptest.NewRequest("POST", "http://localhost/api/auth/login", strings.NewReader(string(body)))
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
}
func TestLoginRateLimitAndMissingKeyFailClosed(t *testing.T) {
	a := &accessControl{sessions: map[[32]byte]time.Time{}, attempts: map[string]loginAttempt{}}
	handler := a.handler(http.NotFoundHandler())
	for index := 0; index < 6; index++ {
		r := httptest.NewRequest("POST", "http://localhost/api/auth/login", strings.NewReader(`{"key":"wrong"}`))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		expected := 401
		if index == 5 {
			expected = 429
		}
		if w.Code != expected {
			t.Fatal("rate limit", index, w.Code)
		}
	}
	w := httptest.NewRecorder()
	New(nil, fstest.MapFS{}, "").ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/api/overview", nil))
	if w.Code != 503 {
		t.Fatal("empty key disabled protection")
	}
}

func TestSecureCookieAndExpiredSession(t *testing.T) {
	handler := New(nil, fstest.MapFS{}, "webcp-secure-cookie-test-key", true)
	request := httptest.NewRequest("POST", "https://localhost/api/auth/login", strings.NewReader(`{"key":"webcp-secure-cookie-test-key"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !response.Result().Cookies()[0].Secure {
		t.Fatal("secure cookie not set")
	}
	control := &accessControl{sessions: map[[32]byte]time.Time{sha256.Sum256([]byte("expired")): time.Now().Add(-time.Minute)}, attempts: map[string]loginAttempt{}}
	request = httptest.NewRequest("GET", "http://localhost/api/overview", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: "expired"})
	response = httptest.NewRecorder()
	control.handler(http.NotFoundHandler()).ServeHTTP(response, request)
	if response.Code != 401 || len(control.sessions) != 0 {
		t.Fatal("expired session accepted")
	}
}
