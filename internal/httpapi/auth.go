package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"io/fs"
	"web-panel/internal/service"
)

const sessionCookie = "webcp_session"
const sessionLifetime = 12 * time.Hour

type loginAttempt struct {
	Count int
	Until time.Time
}
type accessControl struct {
	key          [32]byte
	secureCookie bool
	mu           sync.Mutex
	sessions     map[[32]byte]time.Time
	attempts     map[string]loginAttempt
}

// New requires an injected key. Only static login assets and the auth endpoints
// are public; every management API, download and upload requires a session.
func New(manager *service.Manager, assets fs.FS, key string, secureCookie ...bool) http.Handler {
	control := &accessControl{key: sha256.Sum256([]byte(key)), sessions: map[[32]byte]time.Time{}, attempts: map[string]loginAttempt{}}
	control.secureCookie = len(secureCookie) > 0 && secureCookie[0]
	if len(key) < 16 || len(key) > 4096 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 503, map[string]string{"error": "WebCP 访问密钥尚未配置"})
		})
	}
	return http.NewCrossOriginProtection().Handler(control.handler(newAPI(manager, assets)))
}
func (a *accessControl) valid(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	id := sha256.Sum256([]byte(cookie.Value))
	a.mu.Lock()
	defer a.mu.Unlock()
	until, exists := a.sessions[id]
	if !exists {
		return false
	}
	if !time.Now().Before(until) {
		delete(a.sessions, id)
		return false
	}
	return true
}
func (a *accessControl) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		switch r.URL.Path {
		case "/api/auth/status":
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			writeJSON(w, 200, map[string]bool{"authenticated": a.valid(r)})
			return
		case "/api/auth/login":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			a.login(w, r)
			return
		case "/api/auth/logout":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			if cookie, err := r.Cookie(sessionCookie); err == nil {
				a.mu.Lock()
				delete(a.sessions, sha256.Sum256([]byte(cookie.Value)))
				a.mu.Unlock()
			}
			http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secureCookie || r.TLS != nil})
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if (r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")) && !a.valid(r) {
			writeJSON(w, 401, map[string]string{"error": "请先使用访问密钥登录"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *accessControl) login(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	now := time.Now()
	a.mu.Lock()
	for address, attempt := range a.attempts {
		if now.After(attempt.Until) {
			delete(a.attempts, address)
		}
	}
	attempt := a.attempts[host]
	if (attempt.Count >= 5 && now.Before(attempt.Until)) || (attempt.Count == 0 && len(a.attempts) >= 4096) {
		a.mu.Unlock()
		w.Header().Set("Retry-After", "300")
		writeJSON(w, 429, map[string]string{"error": "尝试次数过多，请稍后重试"})
		return
	}
	attempt.Count++
	if attempt.Until.IsZero() {
		attempt.Until = now.Add(5 * time.Minute)
	}
	a.attempts[host] = attempt
	a.mu.Unlock()

	var body struct {
		Key string `json:"key"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "无效登录请求"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "无效登录请求"})
		return
	}
	candidate := sha256.Sum256([]byte(body.Key))
	if subtle.ConstantTimeCompare(candidate[:], a.key[:]) != 1 {

		writeJSON(w, 401, map[string]string{"error": "访问密钥不正确"})
		return
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		writeJSON(w, 500, map[string]string{"error": "无法创建登录会话"})
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(token)
	a.mu.Lock()
	delete(a.attempts, host)
	for id, until := range a.sessions {
		if now.After(until) {
			delete(a.sessions, id)
		}
	}
	if len(a.sessions) >= 1024 {
		a.mu.Unlock()
		writeJSON(w, 503, map[string]string{"error": "登录会话已满，请稍后重试"})
		return
	}
	a.sessions[sha256.Sum256([]byte(encoded))] = now.Add(sessionLifetime)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: encoded, Path: "/", MaxAge: int(sessionLifetime.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secureCookie || r.TLS != nil})
	writeJSON(w, 200, map[string]bool{"authenticated": true})
}
