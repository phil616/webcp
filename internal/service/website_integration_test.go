package service

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	"web-panel/internal/openresty"
	"web-panel/internal/pki"
	"web-panel/internal/store"
)

func TestWebsiteSettingsIntegration(t *testing.T) {
	integrationOnly(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var hits atomic.Int64
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/origin/socket" {
			conn, buffer, e := w.(http.Hijacker).Hijack()
			if e != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: upgrade\r\nUpgrade: websocket\r\n\r\n")
			buffer.Flush()
			io.Copy(conn, buffer)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/cache/") {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<p>cache-%d before</p>", hits.Add(1))
			return
		}
		if r.URL.Path == "/origin/error" {
			w.Header().Set("Access-Control-Allow-Origin", "https://wrong.test")
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"uri": r.URL.RequestURI(), "host": r.Host, "ip": r.Header.Get("X-Real-IP"), "forwarded": r.Header.Get("X-Forwarded-For")})
	}))
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	backend.Listener = listener
	backend.Start()
	defer backend.Close()
	origin := "http://host.docker.internal:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "resty-settings-" + domain.NewID()[:10]
	httpPort, tlsPort := freePort(t), freePort(t)
	d := &openresty.Docker{Root: s.Root, Container: name, Image: "1panel/openresty:1.27.1.2-5-1-focal", HTTPBind: "127.0.0.1:" + httpPort, HTTPSBind: "127.0.0.1:" + tlsPort}
	t.Cleanup(func() {
		out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=resty.manager.root="+s.Root).Output()
		for _, id := range strings.Fields(string(out)) {
			exec.Command("docker", "rm", "-f", id).Run()
		}
	})
	m, err := New(s, d)
	if err != nil {
		t.Fatal(err)
	}
	site, err := m.SaveSite(revisionOf(t, s), domain.Site{Name: "settings", Kind: "static", Domains: []string{"site.test", "alias.test"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for filename, content := range map[string]string{"public/home.htm": "primary-document", "public/index.html": "secondary-document"} {
		if err = m.WriteFile(revisionOf(t, s), site.ID, filename, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.SaveDirectory(revisionOf(t, s), site.ID, "/public"); err != nil {
		t.Fatal(err)
	}
	if err = m.SaveIndex(revisionOf(t, s), site.ID, []string{"absent.html", "home.htm", "index.html"}); err != nil {
		t.Fatal(err)
	}
	cors := domain.CORSConfig{Enabled: true, AllowOrigins: "https://one.test\nhttps://two.test", AllowMethods: "GET,POST,OPTIONS", AllowHeaders: "Content-Type,Authorization", AllowCredentials: true, Preflight: true}
	if err = m.SaveCORS(revisionOf(t, s), site.ID, cors); err != nil {
		t.Fatal(err)
	}
	proxy, err := m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "api", Enabled: true, Match: "/api/", ProxyPass: origin + "/origin/", ProxyHost: "origin.test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "cached", Enabled: true, Match: "/cached/", ProxyPass: origin + "/cache/", ProxyHost: "$host", Cache: true, ServerCacheTime: 10, ServerCacheUnit: "m", Replaces: []domain.Replacement{{From: "before", To: "after"}}, CacheTime: 1, CacheUnit: "h"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "regex", Enabled: true, Modifier: "~*", Match: "^/UPPER/[a-z]+$", ProxyPass: origin, ProxyHost: "$proxy_host", CORS: domain.CORSConfig{Enabled: true, AllowOrigins: "https://private.test", AllowMethods: "GET"}})
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		if e := m.Apply(ctx, revisionOf(t, s)); e != nil {
			t.Fatal(e)
		}
	}
	httpClient := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "http://127.0.0.1:" + httpPort
	type result struct {
		code   int
		body   string
		header http.Header
		err    error
	}
	fetch := func(method, base, path, host string, headers map[string]string) result {
		req, _ := http.NewRequest(method, base+path, nil)
		req.Host = host
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, e := httpClient.Do(req)
		if e != nil {
			return result{err: e}
		}
		defer res.Body.Close()
		body, e := io.ReadAll(res.Body)
		return result{res.StatusCode, string(body), res.Header, e}
	}
	check := func(method, base, path, host string, headers map[string]string, predicate func(result) bool) result {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			r := fetch(method, base, path, host, headers)
			if predicate(r) {
				return r
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s %s %s: %d %s %v\n%s", method, host, path, r.code, r.body, r.err, dockerCommand(t, "logs", "--tail", "15", name))
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	var sni atomic.Value
	tlsBackend := httptest.NewUnstartedServer(backend.Config.Handler)
	tlsListener, e := net.Listen("tcp", "0.0.0.0:0")
	if e != nil {
		t.Fatal(e)
	}
	tlsBackend.Listener = tlsListener
	tlsBackend.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) { sni.Store(hello.ServerName); return nil, nil }}
	tlsBackend.StartTLS()
	defer tlsBackend.Close()
	tlsOrigin := "https://host.docker.internal:" + strconv.Itoa(tlsListener.Addr().(*net.TCPAddr).Port)
	tlsProxy, e := m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "tls-origin", Enabled: true, Match: "/secure/", ProxyPass: tlsOrigin + "/origin/", ProxyHost: "$host", SNI: true, SSLName: "origin.example.test"})
	if e != nil {
		t.Fatal(e)
	}
	apply()
	check("GET", base, "/secure/item", "site.test", nil, func(r result) bool { return r.code == 200 && sni.Load() == "origin.example.test" })
	tlsProxy.SSLVerify = true
	if _, e = m.SaveProxy(revisionOf(t, s), site.ID, tlsProxy); e != nil {
		t.Fatal(e)
	}
	apply()
	check("GET", base, "/secure/item", "site.test", nil, func(r result) bool { return r.code == 502 })
	tlsProxy.SSLVerify = false
	if _, e = m.SaveProxy(revisionOf(t, s), site.ID, tlsProxy); e != nil {
		t.Fatal(e)
	}
	apply()
	check("GET", base, "/", "site.test", nil, func(r result) bool { return r.code == 200 && r.body == "primary-document" })
	r := check("GET", base, "/api/thing?x=1", "site.test", nil, func(r result) bool {
		return r.code == 200 && strings.Contains(r.body, "/origin/thing?x=1") && strings.Contains(r.body, "origin.test")
	})
	var observed map[string]string
	json.Unmarshal([]byte(r.body), &observed)
	remote := observed["ip"]
	check("GET", base, "/upper/demo?x=1", "site.test", map[string]string{"Origin": "https://private.test"}, func(r result) bool {
		return r.code == 200 && r.header.Get("Access-Control-Allow-Origin") == "https://private.test" && strings.Contains(r.body, "/upper/demo?x=1")
	})
	check("OPTIONS", base, "/api/thing", "site.test", map[string]string{"Origin": "https://two.test"}, func(r result) bool {
		return r.code == 204 && r.header.Get("Access-Control-Allow-Origin") == "https://two.test" && r.header.Get("Access-Control-Allow-Credentials") == "true"
	})
	check("GET", base, "/api/thing", "site.test", map[string]string{"Origin": "https://evil.test"}, func(r result) bool { return r.code == 200 && r.header.Get("Access-Control-Allow-Origin") == "" })
	check("GET", base, "/api/error", "site.test", map[string]string{"Origin": "https://one.test"}, func(r result) bool {
		return r.code == 404 && r.header.Get("Access-Control-Allow-Origin") == "https://one.test"
	})
	// WebSocket upgrade keeps the connection bidirectional through the proxy.
	conn, e := net.DialTimeout("tcp", "127.0.0.1:"+httpPort, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprint(conn, "GET /api/socket HTTP/1.1\r\nHost: site.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	reader := bufio.NewReader(conn)
	line, e := reader.ReadString('\n')
	if e != nil || !strings.Contains(line, "101") {
		conn.Close()
		t.Fatalf("upgrade failed: %q %v", line, e)
	}
	for {
		line, e = reader.ReadString('\n')
		if e != nil {
			t.Fatal(e)
		}
		if line == "\r\n" {
			break
		}
	}
	payload := []byte("upgraded\x00connection")
	conn.Write(payload)
	answer := make([]byte, len(payload))
	_, e = io.ReadFull(reader, answer)
	conn.Close()
	if e != nil || string(answer) != string(payload) {
		t.Fatalf("upgrade data: %q %v", answer, e)
	}
	cached := check("GET", base, "/cached/item", "site.test", nil, func(r result) bool {
		return r.code == 200 && strings.Contains(r.body, "after") && r.header.Get("Expires") != ""
	})
	r = fetch("GET", base, "/cached/item", "site.test", nil)
	if r.body != cached.body {
		t.Fatalf("cache missed: %s %s", cached.body, r.body)
	}
	if err = m.ClearProxyCache(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	check("GET", base, "/cached/item", "site.test", nil, func(r result) bool { return r.code == 200 && r.body != cached.body })
	if err = m.SaveRealIP(revisionOf(t, s), site.ID, domain.RealIPConfig{Enabled: true, From: []string{"192.0.2.0/24"}, Header: "X-Forwarded-For"}); err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/api/thing", "site.test", map[string]string{"X-Forwarded-For": "203.0.113.9"}, func(r result) bool { return r.code == 200 && !strings.Contains(r.body, `"ip":"203.0.113.9"`) })
	if err = m.SaveRealIP(revisionOf(t, s), site.ID, domain.RealIPConfig{Enabled: true, From: []string{remote, "198.51.100.1"}, Header: "X-Forwarded-For"}); err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/api/thing", "site.test", map[string]string{"X-Forwarded-For": "203.0.113.9, 198.51.100.1"}, func(r result) bool { return r.code == 200 && strings.Contains(r.body, `"ip":"203.0.113.9"`) })
	bad, e := m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "bad-regex", Enabled: true, Modifier: "~", Match: "(", ProxyPass: origin, ProxyHost: "$host"})
	if e != nil {
		t.Fatal(e)
	}
	previous := d.Status(ctx).AppliedRevision
	if e = m.Apply(ctx, revisionOf(t, s)); e == nil {
		t.Fatal("invalid PCRE applied")
	}
	if d.Status(ctx).AppliedRevision != previous {
		t.Fatal("invalid PCRE changed deployment")
	}
	check("GET", base, "/", "site.test", nil, func(r result) bool { return r.code == 200 && r.body == "primary-document" })
	if e = m.DeleteProxy(revisionOf(t, s), site.ID, bad.ID); e != nil {
		t.Fatal(e)
	}
	_, err = m.SaveRedirect(revisionOf(t, s), site.ID, domain.RedirectRule{Name: "alias", Enabled: true, Type: "domain", Domains: []string{"alias.test"}, Redirect: 301, Target: "https://destination.test", KeepPath: true})
	if err != nil {
		t.Fatal(err)
	}
	pathRule, err := m.SaveRedirect(revisionOf(t, s), site.ID, domain.RedirectRule{Name: "old", Enabled: true, Type: "path", Path: "/old/", Redirect: 302, Target: "https://destination.test/new/", KeepPath: true})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := m.SaveRedirect(revisionOf(t, s), site.ID, domain.RedirectRule{Name: "404", Enabled: true, Type: "404", Redirect: 302, RedirectRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/foo?x=1", "alias.test", nil, func(r result) bool {
		return r.code == 301 && r.header.Get("Location") == "https://destination.test/foo?x=1"
	})
	check("GET", base, "/old/rest?x=1", "site.test", nil, func(r result) bool {
		return r.code == 302 && r.header.Get("Location") == "https://destination.test/new/rest?x=1"
	})
	check("GET", base, "/missing", "site.test", nil, func(r result) bool { return r.code == 302 && strings.HasSuffix(r.header.Get("Location"), "/") })
	pathRule.KeepPath = false
	_, err = m.SaveRedirect(revisionOf(t, s), site.ID, pathRule)
	if err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/old/rest?x=1", "site.test", nil, func(r result) bool {
		return r.code == 302 && r.header.Get("Location") == "https://destination.test/new/"
	})
	if err = m.DeleteRedirect(revisionOf(t, s), site.ID, missing.ID); err != nil {
		t.Fatal(err)
	}
	// The same hostname can serve a different website on a distinct custom port.
	customPort, _ := strconv.Atoi(freePort(t))
	_, err = m.SaveSite(revisionOf(t, s), domain.Site{Name: "other port", Kind: "static", Bindings: []domain.DomainBinding{{Domain: "site.test", Port: customPort}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", "http://127.0.0.1:"+strconv.Itoa(customPort), "/", "site.test", nil, func(r result) bool { return r.code == 200 && strings.Contains(r.body, "It works") })
	ca, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Root", Kind: "ca", Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	cert, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Server", Kind: "server", CAID: ca.ID, Domains: site.Domains, Days: 5})
	if err != nil {
		t.Fatal(err)
	}
	cfg := domain.DefaultHTTPS()
	cfg.HTTPConfig = "HTTPAlso"
	cfg.HSTS = true
	cfg.HSTSIncludeSubDomains = true
	cfg.HTTP3 = true
	if err = m.SaveHTTPS(revisionOf(t, s), site.ID, HTTPSUpdate{TLSID: cert.ID, HTTPS: cfg}); err != nil {
		t.Fatal(err)
	}
	apply()
	snapshot, _ := s.Load()
	pool := x509.NewCertPool()
	for _, c := range snapshot.Certificates {
		if c.ID == ca.ID {
			pool.AppendCertsFromPEM([]byte(c.CertPEM))
		}
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "site.test", MinVersion: tls.VersionTLS12}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	httpClient.Transport = transport
	secure := "https://127.0.0.1:" + tlsPort
	check("GET", secure, "/api/thing", "site.test", map[string]string{"Origin": "https://one.test"}, func(r result) bool {
		return r.code == 200 && strings.Contains(r.header.Get("Strict-Transport-Security"), "includeSubDomains") && strings.Contains(r.header.Get("Alt-Svc"), ":"+tlsPort) && r.header.Get("Access-Control-Allow-Origin") == "https://one.test"
	})
	check("GET", base, "/", "site.test", nil, func(r result) bool { return r.code == 200 })
	// Uploading a replacement keeps the site's record binding and reloads live TLS automatically.
	replacement, e := issueSSLFixture(t, m, pki.IssueRequest{Name: "Replacement", Kind: "server", CAID: ca.ID, Domains: site.Domains, Days: 6})
	if e != nil {
		t.Fatal(e)
	}
	snapshot, _ = s.Load()
	var sslID uint
	for _, record := range snapshot.SSL.Records {
		if record.MaterialID == cert.ID {
			sslID = record.ID
		}
	}
	if e = m.SSLUpload(SSLCommand{SSLID: sslID, Certificate: replacement.CertPEM, WebsiteSSL: model.WebsiteSSL{Type: "paste", PrivateKey: replacement.KeyPEM}}); e != nil {
		t.Fatal(e)
	}
	servedNewCertificate := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", "127.0.0.1:"+tlsPort, transport.TLSClientConfig)
		if e == nil {
			servedNewCertificate = conn.ConnectionState().PeerCertificates[0].SerialNumber.String() == replacement.Serial
			conn.Close()
			if servedNewCertificate {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !servedNewCertificate {
		t.Fatal("live OpenResty still serves the old uploaded certificate")
	}
	cfg.HTTPConfig = "HTTPToHTTPS"
	if err = m.SaveHTTPS(revisionOf(t, s), site.ID, HTTPSUpdate{TLSID: cert.ID, HTTPS: cfg}); err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/foo?x=1", "site.test", nil, func(r result) bool {
		return r.code == 301 && r.header.Get("Location") == "https://site.test:"+tlsPort+"/foo?x=1"
	})
	cfg.HTTPConfig = "HTTPSOnly"
	if err = m.SaveHTTPS(revisionOf(t, s), site.ID, HTTPSUpdate{TLSID: cert.ID, HTTPS: cfg}); err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", base, "/", "site.test", nil, func(r result) bool { return r.err != nil })
	proxy.Enabled = false
	if _, err = m.SaveProxy(revisionOf(t, s), site.ID, proxy); err != nil {
		t.Fatal(err)
	}
	apply()
	check("GET", secure, "/api/thing", "site.test", nil, func(r result) bool { return r.code == 404 })
	record, err := m.Backup(ctx, revisionOf(t, s), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteProxy(revisionOf(t, s), site.ID, proxy.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(ctx, revisionOf(t, s), record.ID, false); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.Load()
	if snapshot.Sites[0].RunDirectory != "/public" || len(snapshot.Sites[0].Proxies) != 4 || !snapshot.Sites[0].HTTPS.HTTP3 || len(snapshot.Sites[0].Redirects) != 2 {
		t.Fatal("website settings lost in backup")
	}
}
