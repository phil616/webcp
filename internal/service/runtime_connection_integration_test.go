package service

import (
	"context"
	"fmt"
	"github.com/go-acme/lego/v5/providers/http/webroot"
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

func TestExistingContainerConnectionIntegration(t *testing.T) {
	integrationOnly(t)
	ctx := context.Background()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefix := "webcp-connect-" + domain.NewID()[:10]
	image := "1panel/openresty:1.27.1.2-5-1-focal"
	initial := openresty.Connection{Mode: "managed", Container: prefix + "-managed", Image: image, HTTPBind: "127.0.0.1:" + freePort(t), HTTPSBind: "127.0.0.1:" + freePort(t)}
	controller, err := openresty.NewController(s.Root, initial, s)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, controller)
	if err != nil {
		t.Fatal(err)
	}
	site := createSite(t, m, s)
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", initial.Container, prefix+"-custom").Run() })
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	external := initial
	external.Mode = "existing"
	external.Container = prefix + "-custom"
	external.HTTPBind = "127.0.0.1:" + freePort(t)
	external.HTTPSBind = "127.0.0.1:" + freePort(t)
	args := []string{"create", "--name", external.Container, "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--entrypoint", "/usr/local/openresty/bin/openresty", "--mount", "type=bind,src=" + filepath.Join(s.Root, "runtime") + ",dst=" + openresty.Mount + ",readonly", "--mount", "type=bind,src=" + filepath.Join(s.Root, "sites") + ",dst=" + openresty.Mount + "/sites,readonly", "--mount", "type=bind,src=" + filepath.Join(s.Root, "cache") + ",dst=" + openresty.Mount + "/cache", "-p", external.HTTPBind + ":80", "-p", external.HTTPSBind + ":443", image, "-c", openresty.Mount + "/current/nginx.conf", "-g", "daemon off;"}
	id := dockerCommand(t, args...)
	dockerCommand(t, "start", external.Container)
	// The container has no WebCP ownership labels.
	probe, err := controller.Probe(ctx, external)
	if err != nil || !probe.Compatible {
		t.Fatalf("probe: %+v %v", probe, err)
	}
	if err = m.ConfigureRuntime(ctx, controller.Connection().Version, external); err != nil {
		t.Fatal(err)
	}
	if controller.Connection().ContainerID != id {
		t.Fatal("external identity not pinned")
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal(err)
	}
	if controller.Status(ctx).Container != external.Container || !controller.Status(ctx).Running {
		t.Fatal("external container was not detected")
	}
	if dockerCommand(t, "inspect", "--format", "{{.State.Running}}", initial.Container) != "true" {
		t.Fatal("switching connection stopped the previous container")
	}
	if err = m.WriteFile(revisionOf(t, s), site.ID, "probe.txt", []byte("external file binding")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		request, _ := http.NewRequest("GET", "http://"+external.HTTPBind+"/probe.txt", nil)
		request.Host = site.Domains[0]
		response, e := (&http.Client{Timeout: time.Second}).Do(request)
		if e == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if string(body) == "external file binding" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("existing container did not serve shared website files", e)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err = m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal("restart existing container:", err)
	}
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", external.Container) != id {
		t.Fatal("external container was recreated")
	}
	reloaded, err := openresty.NewController(s.Root, initial, s)
	if err != nil || reloaded.Connection().ContainerID != id {
		t.Fatal("connection selection was not persisted", err)
	}
}

func TestUnpreparedContainerTakeoverIntegration(t *testing.T) {
	integrationOnly(t)
	ctx := context.Background()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "webcp-takeover-" + domain.NewID()[:10]
	image := "1panel/openresty:1.27.1.2-5-1-focal"
	httpPort := freePort(t)
	httpsPort := freePort(t)
	id := dockerCommand(t, "run", "-d", "--name", name, "-p", "127.0.0.1:"+httpPort+":80", "-p", "127.0.0.1:"+httpsPort+":443", image)
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() })
	connection := openresty.Connection{Mode: "managed", Container: "unused-managed", Image: image, HTTPBind: "127.0.0.1:" + httpPort, HTTPSBind: "127.0.0.1:" + httpsPort}
	controller, err := openresty.NewController(s.Root, connection, s)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, controller)
	if err != nil {
		t.Fatal(err)
	}
	site := createSite(t, m, s)
	connection.Mode = "existing"
	connection.Container = name
	if err = m.ConfigureRuntime(ctx, controller.Connection().Version, connection); err != nil {
		t.Fatal(err)
	}
	selected := controller.Connection()
	if selected.BoundData {
		t.Fatal("unprepared container incorrectly detected as mounted")
	}
	original := dockerCommand(t, "exec", id, "cat", selected.MainConfig)
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal("takeover:", err)
	}
	backup, err := os.ReadFile(filepath.Join(s.Root, "runtime/external", id, "original-nginx.conf"))
	if err != nil || strings.TrimSpace(string(backup)) != original {
		t.Fatal("original configuration was not backed up", err)
	}
	get := func(name string) (int, string, error) {
		request, _ := http.NewRequest("GET", "http://127.0.0.1:"+httpPort+name, nil)
		request.Host = site.Domains[0]
		response, e := (&http.Client{Timeout: time.Second}).Do(request)
		if e != nil {
			return 0, "", e
		}
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		return response.StatusCode, string(body), e
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, body, e := get("/")
		if e == nil && status == 200 && strings.Contains(body, "It works") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("takeover not serving site: %d %s %v", status, body, e)
		}
		time.Sleep(50 * time.Millisecond)
	}
	beforePages := revisionOf(t, s)
	generationBefore, _ := os.Readlink(filepath.Join(s.Root, "runtime/current"))
	if err = m.SavePages(beforePages, map[string]string{"404": "external page file live"}); err != nil {
		t.Fatal(err)
	}
	if status, body, e := get("/missing-page"); e != nil || status != 404 || body != "external page file live" {
		t.Fatalf("instant page sync: %d %s %v", status, body, e)
	}
	if err = m.savePageFiles(beforePages, site.ID, "", map[string]string{"404": "copied override"}); err != nil {
		t.Fatal(err)
	}
	if status, body, e := get("/missing-page"); e != nil || status != 404 || body != "copied override" {
		t.Fatalf("copied override: %d %q %v", status, body, e)
	}
	if err = m.savePageFiles(beforePages, site.ID, "", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if status, body, e := get("/missing-page"); e != nil || status != 404 || body != "external page file live" {
		t.Fatalf("deleted override did not inherit global page: %d %q %v", status, body, e)
	}
	generationAfter, _ := os.Readlink(filepath.Join(s.Root, "runtime/current"))
	if revisionOf(t, s) != beforePages || generationAfter != generationBefore {
		t.Fatal("page file edit changed controller config or deployed a generation")
	}
	if err = m.WriteFile(revisionOf(t, s), site.ID, "live.txt", []byte("copied immediately")); err != nil {
		t.Fatal(err)
	}
	if status, body, e := get("/live.txt"); e != nil || status != 200 || body != "copied immediately" {
		t.Fatalf("file sync: %d %s %v", status, body, e)
	}
	if err = m.DeleteFile(revisionOf(t, s), site.ID, "live.txt"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := get("/live.txt"); status != 404 {
		t.Fatal("deleted file still served")
	}
	provider, e := webroot.NewHTTPProvider(filepath.Join(s.Root, "runtime/acme"))
	if e != nil {
		t.Fatal(e)
	}
	challenge := &syncedHTTPChallenge{provider: provider, sync: func(ctx context.Context, name string, remove bool) error {
		return m.syncRuntimeFile(ctx, "acme", "", name, remove)
	}}
	if e = challenge.Present(ctx, site.Domains[0], "webcp-token", "webcp-proof"); e != nil {
		t.Fatal(e)
	}
	if status, body, e := get("/.well-known/acme-challenge/webcp-token"); e != nil || status != 200 || body != "webcp-proof" {
		t.Fatalf("HTTP-01 sync: %d %s %v", status, body, e)
	}
	if e = challenge.CleanUp(ctx, site.Domains[0], "webcp-token", "webcp-proof"); e != nil {
		t.Fatal(e)
	}
	if status, _, _ := get("/.well-known/acme-challenge/webcp-token"); status != 404 {
		t.Fatal("HTTP-01 token cleanup failed")
	}
	if err = m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err != nil {
		t.Fatal("restart:", err)
	}
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", name) != id {
		t.Fatal("existing container was recreated")
	}
}
