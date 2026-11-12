package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/service"
	"web-panel/internal/store"
)

type runtimeStub struct{}

func (runtimeStub) Status(context.Context) openresty.Status           { return openresty.Status{} }
func (runtimeStub) Apply(context.Context, openresty.Generation) error { return nil }
func (runtimeStub) Stop(context.Context) error                        { return nil }
func (runtimeStub) Logs(context.Context) (string, error)              { return "", nil }

func TestStrictRequestsAndCrossOriginProtection(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.New(s, runtimeStub{})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SSLCreateCA(service.CACreate{Name: "Root", CommonName: "Root", Country: "CN", Organization: "Test", KeyType: "EC256"}); err != nil {
		t.Fatal(err)
	}
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("embedded-ui")}})
	for _, test := range []struct {
		method, path, body, revision, origin string
		status                               int
	}{{"GET", "/", "", "", "", 200}, {"GET", "/api/overview", "", "", "", 200}, {"POST", "/api/sites", `{"name":"x","php":true}`, "2", "", 400}, {"POST", "/api/sites", `{}`, "", "", 400}, {"PUT", "/api/pages", `{"pages":{"404":"updated"},"etag":"stale"}`, "1", "", 409}, {"PUT", "/api/pages", `{"pages":{"404":"updated"},"etag":"stale"}`, "2", "https://evil.test", 403}} {
		r := httptest.NewRequest(test.method, "http://localhost"+test.path, strings.NewReader(test.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", test.revision)
		if test.origin != "" {
			r.Header.Set("Origin", test.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("%s %s: %d: %s", test.method, test.path, w.Code, w.Body.String())
		}
		if test.path == "/api/overview" {
			var v map[string]any
			if err = json.Unmarshal(w.Body.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(w.Body.String(), "PRIVATE KEY") || strings.Contains(w.Body.String(), "keyPem") {
				t.Fatal("private key leaked in overview")
			}
		}
	}
	r := httptest.NewRequest(http.MethodGet, "http://localhost/api/not-found", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal("API fallback must return JSON 404")
	}
}

func TestStreamAPI(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.New(s, runtimeStub{})
	if err != nil {
		t.Fatal(err)
	}
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("ui")}})
	send := func(method, path, body, rev string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, "http://localhost/api"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", rev)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	send("POST", "/streams", `{"name":"invalid","loadBalance":true}`, "1", 400)
	send("POST", "/streams", `{}`, "", 400)
	data := send("POST", "/streams", `{"name":"ssh","protocol":"tcp","listenPort":2222,"targetHost":"example.test","targetPort":22,"enabled":true}`, "1", 200)
	var proxy struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &proxy); err != nil {
		t.Fatal(err)
	}
	send("DELETE", "/streams/"+proxy.ID, "", "1", 409)
	send("DELETE", "/streams/"+proxy.ID, "", "2", 200)
	send("DELETE", "/streams/"+proxy.ID, "", "3", 404)
}

func TestWebsiteSectionRoutes(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := service.New(s, runtimeStub{})
	if err != nil {
		t.Fatal(err)
	}
	site, err := m.SaveSite(1, domain.Site{Name: "test", Kind: "static", Domains: []string{"test.example"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("ui")}})
	send := func(method, section, body, rev string, status int) {
		t.Helper()
		r := httptest.NewRequest(method, "http://localhost/api/sites/"+site.ID+"/"+section, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", rev)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", section, w.Code, w.Body.String())
		}
	}
	send("PUT", "domains", `{"bindings":[]}`, "2", 400)
	send("PUT", "domains", `{"bindings":[{"domain":"x.test","port":80,"ssl":false}],"php":true}`, "2", 400)
	send("PUT", "index", `{"index":["home.html","index.html"]}`, "2", 200)
	send("PUT", "cors", `{"enabled":false}`, "2", 409)
	send("PUT", "real-ip", `{"enabled":true,"from":["127.0.0.1"],"header":"CF-Connecting-IP"}`, "3", 200)
	document, _ := m.ReadPages(site.ID)
	body, _ := json.Marshal(map[string]any{"pages": map[string]string{"404": "custom"}, "etag": document.ETag})
	send("PUT", "pages", string(body), "4", 200)
	send("GET", "directories", "", "", 200)
	send("POST", "proxies", `{"name":"api","match":"/api/","proxyPass":"http://127.0.0.1:9000","enabled":true}`, "4", 200)
	snapshot, _ := s.Load()
	if snapshot.Sites[0].Index[0] != "home.html" || snapshot.Sites[0].RealIP.Header != "CF-Connecting-IP" || func() bool {
		document, e := m.ReadPages(snapshot.Sites[0].ID)
		return e != nil || document.Pages["404"] != "custom"
	}() {
		t.Fatal("section persistence failed")
	}
}

func TestOpenRestyPerformanceAPI(t *testing.T) {
	s, e := store.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	m, e := service.New(s, runtimeStub{})
	if e != nil {
		t.Fatal(e)
	}
	h := newAPI(m, fstest.MapFS{"index.html": {Data: []byte("ui")}})
	send := func(path, body, rev string, status int) string {
		t.Helper()
		r := httptest.NewRequest("POST", "http://localhost/api/panel/openresty/"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", rev)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	send("update", `{"scope":"http-per","operate":"update","params":{"gzip":"on","gzip_comp_level":"3"}}`, "1", 200)
	text := send("scope", `{"scope":"http-per"}`, "2", 200)
	if !strings.Contains(text, `"on"`) {
		t.Fatal("updated settings missing", text)
	}
	send("update", `{"scope":"http-per","operate":"update","params":{"gzip":"off"}}`, "1", 409)
	send("update", `{"scope":"http-per","operate":"update","params":{"include":"/tmp/file"}}`, "2", 400)
	send("update", `{"scope":"http-per","operate":"update","params":{"gzip":"on"},"unknown":true}`, "2", 400)
}
