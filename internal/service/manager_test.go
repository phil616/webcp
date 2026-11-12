package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
	"web-panel/internal/openresty"
	"web-panel/internal/pki"
	"web-panel/internal/store"
)

type fakeRuntime struct {
	applied uint64
	stopped bool
	fail    bool
}

func (r *fakeRuntime) Status(context.Context) openresty.Status {
	return openresty.Status{Available: true, Running: !r.stopped, AppliedRevision: r.applied}
}
func (r *fakeRuntime) Apply(_ context.Context, g openresty.Generation) error {
	if r.fail {
		return errors.New("invalid config")
	}
	r.applied = g.Revision
	return nil
}
func (r *fakeRuntime) Stop(context.Context) error           { r.stopped = true; return nil }
func (r *fakeRuntime) Logs(context.Context) (string, error) { return "", nil }

func setup(t *testing.T) (*Manager, *store.Store, *fakeRuntime) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &fakeRuntime{}
	m, err := New(s, r)
	if err != nil {
		t.Fatal(err)
	}
	return m, s, r
}
func revisionOf(t *testing.T, s *store.Store) uint64 {
	t.Helper()
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return state.Revision
}
func createSite(t *testing.T, m *Manager, s *store.Store) domain.Site {
	t.Helper()
	site, err := m.SaveSite(revisionOf(t, s), domain.Site{Name: "example", Domains: []string{"example.test"}, Kind: "static", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func TestRevisionsReferencesAndFailedApply(t *testing.T) {
	m, s, r := setup(t)
	site := createSite(t, m, s)
	if _, err := m.SaveSite(1, site); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
	l, err := m.SaveLua(revisionOf(t, s), domain.Lua{Name: "access_block", Kind: "block", Code: `ngx.header["X-Test"] = "yes"`})
	if err != nil {
		t.Fatal(err)
	}
	site.LuaBindings = map[string]string{"access": l.ID}
	if _, err = m.SaveSite(revisionOf(t, s), site); err != nil {
		t.Fatal(err)
	}
	rev := revisionOf(t, s)
	if err = m.DeleteLua(rev, l.ID); err == nil {
		t.Fatal("deleted referenced block")
	}
	if revisionOf(t, s) != rev {
		t.Fatal("failed transaction changed revision")
	}
	if err = m.Apply(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	r.fail = true
	if err = m.SavePages(rev, map[string]string{"404": "new page"}); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(context.Background(), revisionOf(t, s)); err == nil {
		t.Fatal("expected failure")
	}
	if r.applied != rev {
		t.Fatal("failed deployment changed applied revision")
	}
}

func TestBackupRestoreAndCrashRecovery(t *testing.T) {
	m, s, r := setup(t)
	site := createSite(t, m, s)
	ctx := context.Background()
	if err := m.WriteFile(revisionOf(t, s), site.ID, "assets/page.txt", []byte("before")); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveOpenRestyPerformance(ctx, revisionOf(t, s), map[string]string{"gzip": "on", "keepalive_timeout": "90s"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SavePages(revisionOf(t, s), map[string]string{"495": "invalid certificate", "496": "missing certificate"}); err != nil {
		t.Fatal(err)
	}
	record, err := m.Backup(ctx, revisionOf(t, s), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.WriteFile(revisionOf(t, s), site.ID, "assets/page.txt", []byte("after")); err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteSite(revisionOf(t, s), site.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(ctx, revisionOf(t, s), record.ID, false); err != nil {
		t.Fatal(err)
	}
	content, err := m.ReadFile(site.ID, "assets/page.txt")
	if err != nil || string(content) != "before" {
		t.Fatalf("restore: %q %v", content, err)
	}
	if !r.stopped {
		t.Fatal("restore must stop runtime")
	}
	before, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.OpenResty == nil || before.OpenResty.HTTP["keepalive_timeout"] != "90s" || func() bool {
		document, e := m.ReadPages("")
		return e != nil || document.Pages["496"] != "missing certificate"
	}() {
		t.Fatal("performance settings or certificate error pages lost during restore")
	}
	// Simulate termination after directory replacement and before journal removal.
	j := restoreJournal{Stage: domain.NewID(), OldSites: domain.NewID(), OldPages: domain.NewID(), OldState: before}
	b, _ := marshal(j)
	if err = s.SaveRestoreJournal(b); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(s.Root, "sites"), filepath.Join(s.Root, "restore-old-"+j.OldSites)); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(s.Root, "sites"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(errorpages.New(s.Root).Root, filepath.Join(s.Root, "restore-pages-old-"+j.OldPages)); err != nil {
		t.Fatal(err)
	}
	if err = errorpages.New(s.Root).Ensure(nil); err != nil {
		t.Fatal(err)
	}
	crashed := before
	crashed.Revision++
	if err = s.Save(crashed); err != nil {
		t.Fatal(err)
	}
	if _, err = New(s, r); err != nil {
		t.Fatal(err)
	}
	recovered, e := m.ReadPages("")
	if e != nil || recovered.Pages["496"] != "missing certificate" {
		t.Fatal("page file rollback failed", e)
	}
	if revisionOf(t, s) != before.Revision {
		t.Fatal("SQLite rollback failed")
	}
	content, err = m.ReadFile(site.ID, "assets/page.txt")
	if err != nil || string(content) != "before" {
		t.Fatalf("journal recovery: %q %v", content, err)
	}
}

func TestFilesCannotEscapeRoot(t *testing.T) {
	m, s, _ := setup(t)
	site := createSite(t, m, s)
	for _, name := range []string{"../state.json", "/etc/passwd", "index.PHP", ".hidden/file", "assets/../../bad"} {
		if err := m.WriteFile(revisionOf(t, s), site.ID, name, []byte("bad")); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.Root, "sites", site.ID, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile(revisionOf(t, s), site.ID, "escape/file", []byte("bad")); err == nil {
		t.Fatal("followed symlink outside root")
	}
}

func TestWildcardWebsiteRequiresWildcardCertificate(t *testing.T) {
	m, s, _ := setup(t)
	ca, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Root", Kind: "ca", Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Exact host", Kind: "server", CAID: ca.ID, Domains: []string{"wildcard.example.test"}, Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	site := domain.Site{Name: "Wildcard site", Kind: "static", Domains: []string{"*.example.test"}, Enabled: true, TLSID: leaf.ID}
	if _, err = m.SaveSite(revisionOf(t, s), site); err == nil {
		t.Fatal("exact hostname certificate accepted for a wildcard website")
	}
	wildcard, err := issueSSLFixture(t, m, pki.IssueRequest{Name: "Wildcard", Kind: "server", CAID: ca.ID, Domains: []string{"*.example.test"}, Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	site.TLSID = wildcard.ID
	if _, err = m.SaveSite(revisionOf(t, s), site); err != nil {
		t.Fatal(err)
	}
}

func TestClientIssuanceRecordsSurviveUntilCARetirement(t *testing.T) {
	m, s, _ := setup(t)
	ca, err := m.IssueMTLS(revisionOf(t, s), pki.IssueRequest{Name: "Root", Kind: "ca", Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	client, err := m.IssueMTLS(revisionOf(t, s), pki.IssueRequest{Name: "Client", Kind: "client", CAID: ca.ID, Days: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteMTLSCA(revisionOf(t, s), ca.ID); err == nil {
		t.Fatal("CA retired before client revocation")
	}
	if err = m.RevokeMTLS(revisionOf(t, s), client.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteMTLSCA(revisionOf(t, s), ca.ID); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.MTLS.Authorities) != 0 || len(state.MTLS.Clients) != 0 {
		t.Fatal("retired CA left orphaned issuance records")
	}
}

func TestInvalidRestoreJournalDoesNotMoveDirectories(t *testing.T) {
	m, s, r := setup(t)
	site := createSite(t, m, s)
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	journal := restoreJournal{Stage: domain.NewID(), OldSites: domain.NewID(), OldPages: "../escape", OldState: state}
	old := filepath.Join(s.Root, "restore-old-"+journal.OldSites)
	if err = os.Mkdir(old, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := marshal(journal)
	if err = s.SaveRestoreJournal(data); err != nil {
		t.Fatal(err)
	}
	if _, err = New(s, r); err == nil {
		t.Fatal("invalid journal accepted")
	}
	if _, err = os.Stat(filepath.Join(s.Root, "sites", site.ID, "index.html")); err != nil {
		t.Fatal("invalid journal changed current files", err)
	}
	if _, err = os.Stat(old); err != nil {
		t.Fatal("invalid journal consumed rollback directory", err)
	}
}
