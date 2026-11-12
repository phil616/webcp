package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/store"
)

func TestWebsiteSectionsAreIndependentAndVersioned(t *testing.T) {
	m, s, _ := setup(t)
	site := createSite(t, m, s)
	cors := domain.DefaultCORS()
	cors.Enabled = true
	rev := revisionOf(t, s)
	if err := m.SaveCORS(rev, site.ID, cors); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveIndex(rev, site.ID, []string{"home.html"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if err := m.SaveBindings(revisionOf(t, s), site.ID, []domain.DomainBinding{{Domain: "alias.test", Port: 8081}}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := s.Load()
	current := snapshot.Sites[0]
	if !current.CORS.Enabled || current.Bindings[0].Port != 8081 || current.Domains[0] != "alias.test" || current.Index[0] != "index.html" {
		t.Fatalf("section overwrote unrelated settings: %+v", current)
	}
	before := revisionOf(t, s)
	if err := m.SaveBindings(before, site.ID, nil); err == nil {
		t.Fatal("deleted last binding")
	}
	if revisionOf(t, s) != before {
		t.Fatal("invalid save changed revision")
	}
	if err := m.WriteFile(before, site.ID, "public/home.html", []byte("home")); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveDirectory(before, site.ID, "/public"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(s.Root, "sites", site.ID, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveDirectory(revisionOf(t, s), site.ID, "/escape"); err == nil {
		t.Fatal("followed directory symlink")
	}
	dirs, err := m.Directories(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 || dirs[1] != "/public" {
		t.Fatalf("unsafe directories: %v", dirs)
	}
	proxy, err := m.SaveProxy(revisionOf(t, s), site.ID, domain.ProxyRule{Name: "api", Match: "/api/", ProxyPass: "http://origin.test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	proxy.Name = "rename"
	if _, err = m.SaveProxy(revisionOf(t, s), site.ID, proxy); err == nil {
		t.Fatal("renamed immutable proxy")
	}
	if err = m.DeleteProxy(revisionOf(t, s), site.ID, proxy.ID); err != nil {
		t.Fatal(err)
	}
}
func TestWebsiteAndStreamPublicationsConflict(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := &openresty.Docker{Root: s.Root, Container: "no-docker-needed", HTTPBind: "127.0.0.1:18080", HTTPSBind: "127.0.0.1:18443"}
	m, err := New(s, d)
	if err != nil {
		t.Fatal(err)
	}
	site := createSite(t, m, s)
	if err = m.SaveBindings(revisionOf(t, s), site.ID, []domain.DomainBinding{{Domain: "example.test", Port: 29090}}); err != nil {
		t.Fatal(err)
	}
	p := domain.StreamProxy{Name: "conflict", Protocol: "tcp", ListenAddress: "127.0.0.1", ListenPort: 29090, TargetHost: "127.0.0.1", TargetPort: 22, Enabled: true}
	if _, err = m.SaveStream(revisionOf(t, s), p); err == nil {
		t.Fatal("website and TCP publication overlap accepted")
	}
	p.Protocol = "udp"
	if _, err = m.SaveStream(revisionOf(t, s), p); err != nil {
		t.Fatal(err)
	}
}
