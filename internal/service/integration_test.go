package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/pki"
	"web-panel/internal/store"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}
func dockerCommand(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func integrationOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("RESTY_INTEGRATION") != "1" {
		t.Skip("set RESTY_INTEGRATION=1 to run isolated Docker integration tests")
	}
}

func TestOpenRestyIntegration(t *testing.T) {
	integrationOnly(t)
	ctx := context.Background()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	httpPort, tlsPort := freePort(t), freePort(t)
	name := "resty-test-" + domain.NewID()[:10]
	runtime := &openresty.Docker{Root: s.Root, Image: "1panel/openresty:1.27.1.2-5-1-focal", Container: name, HTTPBind: "127.0.0.1:" + httpPort, HTTPSBind: "127.0.0.1:" + tlsPort}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	m, err := New(s, runtime)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Test Root", Kind: "ca", Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	server, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "localhost", Kind: "server", CAID: ca.ID, Domains: []string{"localhost"}, Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	clientCA, err := m.IssueMTLS(revisionOf(t, s), pki.IssueRequest{Name: "Client Root", Kind: "ca", Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	client, err := m.IssueMTLS(revisionOf(t, s), pki.IssueRequest{Name: "test-client", Kind: "client", CAID: clientCA.ID, Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	module, err := m.SaveLua(revisionOf(t, s), domain.Lua{Name: "test_utils", Kind: "module", Code: `local M = {}; function M.value() return "lua-module-ok" end; return M`})
	if err != nil {
		t.Fatal(err)
	}
	_ = module
	block, err := m.SaveLua(revisionOf(t, s), domain.Lua{Name: "test_access", Kind: "block", Code: `local utils = require("test_utils"); ngx.header["X-Lua-Test"] = utils.value()`})
	if err != nil {
		t.Fatal(err)
	}
	site, err := m.SaveSite(revisionOf(t, s), domain.Site{Name: "mTLS test", Kind: "static", Domains: []string{"localhost"}, Enabled: true, TLSID: server.ID, LuaBindings: map[string]string{"access": block.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SaveMTLSBinding(revisionOf(t, s), domain.MTLSBinding{SiteID: site.ID, CAID: clientCA.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.Root, "sites", site.ID, "secret.pHp"), []byte("PHP source must not leak"), 0600); err != nil {
		t.Fatal(err)
	}
	proxySite, err := m.SaveSite(revisionOf(t, s), domain.Site{Name: "proxy test", Kind: "proxy", Domains: []string{"proxy.test"}, Enabled: true, Upstream: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	streamPort := freePort(t)
	streamNumber := 0
	fmt.Sscan(streamPort, &streamNumber)
	if _, err = m.SaveStream(revisionOf(t, s), domain.StreamProxy{Name: "TLS passthrough", Protocol: "tcp", ListenAddress: "127.0.0.1", ListenPort: streamNumber, TargetHost: "127.0.0.1", TargetPort: 443, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = m.savePageFiles(revisionOf(t, s), site.ID, "", map[string]string{"404": "custom-not-found", "495": "site-invalid-client"}); err != nil {
		t.Fatal(err)
	}
	if err = m.savePageFiles(revisionOf(t, s), proxySite.ID, "", map[string]string{"502": "custom-bad-gateway"}); err != nil {
		t.Fatal(err)
	}
	if err = m.SavePages(revisionOf(t, s), map[string]string{"404": "global-missing", "495": "global-invalid-client", "496": "global-missing-client"}); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	var caFull, clientFull domain.Certificate
	for _, c := range append(state.Certificates, state.MTLS.Clients...) {
		if c.ID == ca.ID {
			caFull = c
		}
		if c.ID == client.ID {
			clientFull = c
		}
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(caFull.CertPEM))
	pair, err := tls.X509KeyPair([]byte(clientFull.CertPEM), []byte(clientFull.KeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	clientConfig := &tls.Config{RootCAs: pool, ServerName: "localhost", Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	get := func(path string, cert bool) (int, string, string, error) {
		cfg := clientConfig.Clone()
		if !cert {
			cfg.Certificates = nil
		}
		transport := &http.Transport{TLSClientConfig: cfg, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		httpClient := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		resp, e := httpClient.Get("https://localhost:" + tlsPort + path)
		if e != nil {
			return 0, "", "", e
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header.Get("X-Lua-Test"), e
	}
	// Wait for the listener, not a fixed startup delay.
	deadline := time.Now().Add(15 * time.Second)
	for {
		status, body, header, e := get("/", true)
		if e == nil && status == 200 {
			if !strings.Contains(body, "It works") || header != "lua-module-ok" {
				t.Fatalf("website or Lua module failed: %s %q", body, header)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("website failed: %d %v %s\n%s", status, e, body, dockerCommand(t, "logs", name))
		}
		time.Sleep(100 * time.Millisecond)
	}
	// TLS and client certificates terminate at the existing HTTP server behind stream.
	transport := &http.Transport{TLSClientConfig: clientConfig.Clone()}
	tlsHTTP := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	passthrough, e := tlsHTTP.Get("https://localhost:" + streamPort + "/")
	if e != nil {
		t.Fatal("TLS passthrough:", e)
	}
	passthrough.Body.Close()
	transport.CloseIdleConnections()
	if passthrough.StatusCode != 200 || passthrough.Header.Get("X-Lua-Test") != "lua-module-ok" {
		t.Fatal("mTLS over stream failed")
	}
	if status, body, _, e := get("/", false); e != nil || status != 403 || body != "global-missing-client" {
		t.Fatalf("missing certificate error page: %d %q %v", status, body, e)
	}
	untrustedCA, e := pki.Issue(pki.IssueRequest{Name: "Untrusted Root", Kind: "ca", Days: 30}, nil)
	if e != nil {
		t.Fatal(e)
	}
	untrustedClient, e := pki.Issue(pki.IssueRequest{Name: "Untrusted", Kind: "client", CAID: untrustedCA.ID, Days: 10}, &untrustedCA)
	if e != nil {
		t.Fatal(e)
	}
	wrongPair, e := tls.X509KeyPair([]byte(untrustedClient.CertPEM), []byte(untrustedClient.KeyPEM))
	if e != nil {
		t.Fatal(e)
	}
	wrongConfig := clientConfig.Clone()
	wrongConfig.Certificates = nil
	wrongConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &wrongPair, nil }
	wrongTransport := &http.Transport{TLSClientConfig: wrongConfig, DisableKeepAlives: true}
	defer wrongTransport.CloseIdleConnections()
	wrongResponse, e := (&http.Client{Transport: wrongTransport, Timeout: 3 * time.Second}).Get("https://localhost:" + tlsPort + "/")
	if e != nil {
		t.Fatal(e)
	}
	wrongBody, e := io.ReadAll(wrongResponse.Body)
	wrongResponse.Body.Close()
	if e != nil || wrongResponse.StatusCode != 403 || string(wrongBody) != "site-invalid-client" {
		t.Fatalf("invalid certificate error page: %d %s %v", wrongResponse.StatusCode, wrongBody, e)
	}
	if err = m.SaveOpenRestyPerformance(ctx, revisionOf(t, s), map[string]string{"gzip": "on", "gzip_min_length": "1", "gzip_comp_level": "3", "client_max_body_size": "20m", "keepalive_timeout": "1m30s 90"}); err != nil {
		t.Fatal(err)
	}
	gzipTransport := &http.Transport{TLSClientConfig: clientConfig.Clone(), DisableCompression: true}
	defer gzipTransport.CloseIdleConnections()
	gzipRequest, _ := http.NewRequest("GET", "https://localhost:"+tlsPort+"/", nil)
	gzipRequest.Header.Set("Accept-Encoding", "gzip")
	deadline = time.Now().Add(10 * time.Second)
	for {
		gzipResponse, e := (&http.Client{Transport: gzipTransport, Timeout: 3 * time.Second}).Do(gzipRequest.Clone(ctx))
		if e != nil {
			t.Fatal(e)
		}
		responseBody, _ := io.ReadAll(gzipResponse.Body)
		gzipResponse.Body.Close()
		if gzipResponse.StatusCode == 200 && gzipResponse.Header.Get("Content-Encoding") == "gzip" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gzip setting not applied: status=%d headers=%v body=%s", gzipResponse.StatusCode, gzipResponse.Header, responseBody)
		}
		gzipTransport.CloseIdleConnections()
		time.Sleep(100 * time.Millisecond)
	}

	if status, body, _, e := get("/missing", true); e != nil || status != 404 || body != "custom-not-found" {
		t.Fatalf("404 page: %d %q %v", status, body, e)
	}
	if status, body, _, e := get("/secret.pHp", true); e != nil || status != 404 || strings.Contains(body, "source must") {
		t.Fatalf("PHP exposure: %d %s %v", status, body, e)
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+httpPort+"/", nil)
	req.Host = "proxy.test"
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 502 || string(body) != "custom-bad-gateway" {
		t.Fatalf("proxy error page: %d %q", res.StatusCode, body)
	}
	if err = m.savePageFiles(revisionOf(t, s), proxySite.ID, "", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	res, e = http.DefaultClient.Do(req.Clone(ctx))
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 502 || string(body) == "custom-bad-gateway" {
		t.Fatalf("missing page must preserve native error status: %d %q", res.StatusCode, body)
	}
	previous := runtime.Status(ctx).AppliedRevision
	block.Code = "function !!! invalid lua"
	if _, err = m.SaveLua(revisionOf(t, s), block); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err == nil {
		t.Fatal("accepted invalid Lua")
	}
	if runtime.Status(ctx).AppliedRevision != previous {
		t.Fatal("invalid Lua switched generation")
	}
	if status, _, _, e := get("/", true); e != nil || status != 200 {
		t.Fatalf("old website disrupted after failed deployment: %d %v", status, e)
	}
	block.Code = `ngx.header["X-Lua-Test"] = "after-reload"`
	if _, err = m.SaveLua(revisionOf(t, s), block); err != nil {
		t.Fatal(err)
	}
	if err = m.RevokeMTLS(revisionOf(t, s), client.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		status, _, _, e := get("/", true)
		if e != nil || status != 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("revoked client still accepted after CRL reload")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err = runtime.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if runtime.Status(ctx).Running {
		t.Fatal("failed to stop owned container")
	}
}

func TestS3Integration(t *testing.T) {
	integrationOnly(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	port := freePort(t)
	name := "resty-minio-" + domain.NewID()[:10]
	dockerCommand(t, "run", "-d", "--rm", "--name", name, "-p", "127.0.0.1:"+port+":9000", "-e", "MINIO_ROOT_USER=resty-test", "-e", "MINIO_ROOT_PASSWORD=resty-test-secret", "minio/minio:RELEASE.2025-09-07T16-13-09Z", "server", "/data")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	endpoint := "127.0.0.1:" + port
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("resty-test", "resty-test-secret", ""), Secure: false})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		err = client.MakeBucket(ctx, "resty-backups", minio.MakeBucketOptions{})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	m, s, _ := setup(t)
	site := createSite(t, m, s)
	if err = m.SaveS3(revisionOf(t, s), domain.S3{Endpoint: "http://" + endpoint, Bucket: "resty-backups", AccessKey: "resty-test", SecretKey: "resty-test-secret", Region: "us-east-1", Prefix: "test//production/", PathStyle: true}); err != nil {
		t.Fatal(err)
	}
	if err = m.CheckS3(ctx); err != nil {
		t.Fatal(err)
	}
	record, err := m.Backup(ctx, revisionOf(t, s), true)
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Backups(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SHA256 != record.SHA256 || items[0].Revision != record.Revision {
		t.Fatalf("bad S3 metadata: %#v %v", items, err)
	}
	if err = m.DeleteSite(revisionOf(t, s), site.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(ctx, revisionOf(t, s), record.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ReadFile(site.ID, "index.html"); err != nil {
		t.Fatal(err)
	}
}
