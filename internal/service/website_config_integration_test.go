package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/store"
)

func TestWebsiteConfigIntegration(t *testing.T) {
	integrationOnly(t)
	ctx := context.Background()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name, port := "resty-config-"+domain.NewID()[:10], freePort(t)
	runtime := &openresty.Docker{Root: s.Root, Container: name, Image: "1panel/openresty:1.27.1.2-5-1-focal", HTTPBind: "127.0.0.1:" + port, HTTPSBind: "127.0.0.1:" + freePort(t)}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	m, err := New(s, runtime)
	if err != nil {
		t.Fatal(err)
	}
	site := createSite(t, m, s)
	root := filepath.Join(s.Root, "sites", site.ID)
	if err = os.WriteFile(filepath.Join(root, ".config-test"), []byte("hidden-ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("index.html", filepath.Join(root, "link.html")); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	check := func(path string, code int, header, body string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		lastStatus := 0
		for {
			req, _ := http.NewRequest("GET", "http://127.0.0.1:"+port+path, nil)
			req.Host, req.Close = "example.test", true
			response, e := client.Do(req)
			if e == nil {
				lastStatus = response.StatusCode
				data, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if response.StatusCode == code && (header == "" || response.Header.Get("X-Config-Test") == header) && strings.Contains(string(data), body) {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s returned status %d; expected %d, header %q, body %q (last error %v)", path, lastStatus, code, header, body, e)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	check("/.config-test", 403, "", "")
	check("/link.html", 404, "", "")
	doc, err := m.WebsiteConfig(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(doc.Content, "server {", "server { add_header X-Config-Test manual always;", 1)
	edited = strings.Replace(edited, "disable_symlinks on;", "disable_symlinks off;", 1)
	edited = strings.Replace(edited, "  location ~ /\\. { deny all; }\n", "", 1)
	if err = m.SaveWebsiteConfig(doc.Revision, site.ID, edited); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	check("/.config-test", 200, "manual", "hidden-ok")
	check("/link.html", 200, "manual", "It works")
	check("/script.php", 404, "", "")
	previous := runtime.Status(ctx).AppliedRevision
	doc, _ = m.WebsiteConfig(site.ID)
	bad := strings.Replace(doc.Content, `"manual"`, `"$webcp_missing_variable"`, 1)
	if err = m.SaveWebsiteConfig(doc.Revision, site.ID, bad); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err == nil {
		t.Fatal("OpenResty accepted unknown header variable")
	}
	if runtime.Status(ctx).AppliedRevision != previous {
		t.Fatal("failed nginx check changed running configuration")
	}
	check("/.config-test", 200, "manual", "hidden-ok")
}
