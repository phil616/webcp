package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"web-panel/internal/domain"
)

func TestWebsiteConfigPersistenceAndGUIUpdates(t *testing.T) {
	m, store, runtime := setup(t)
	site := createSite(t, m, store)
	doc, err := m.WebsiteConfig(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Revision != revisionOf(t, store) {
		t.Fatal("preview and edit revision were not read atomically")
	}
	bad := strings.Replace(doc.Content, "server {", "server { add_trailer X-Test value;", 1)
	if err = m.SaveWebsiteConfig(doc.Revision, site.ID, bad); err == nil || revisionOf(t, store) != doc.Revision || runtime.applied != 0 {
		t.Fatal("rejected configuration changed state or deployment")
	}
	good := strings.Replace(doc.Content, "server {", "server { add_header X-Config value always;", 1)
	if err = m.SaveWebsiteConfig(doc.Revision, site.ID, good); err != nil {
		t.Fatal(err)
	}
	if err = m.SaveWebsiteConfig(doc.Revision, site.ID, good); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted stale edit: %v", err)
	}
	if err = m.SaveIndex(revisionOf(t, store), site.ID, []string{"home.html"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	doc, err = reopened.WebsiteConfig(site.ID)
	if err != nil || !strings.Contains(doc.Content, `add_header X-Config "value" always;`) || !strings.Contains(doc.Content, "index home.html;") {
		t.Fatalf("GUI update lost imported header: %v\n%s", err, doc.Content)
	}
	record, err := m.Backup(context.Background(), doc.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SaveWebsiteAdvanced(revisionOf(t, store), site.ID, WebsiteAdvancedUpdate{Security: domain.WebsiteSecurity{AllowHiddenFiles: true}}); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(context.Background(), revisionOf(t, store), record.ID, false); err != nil {
		t.Fatal(err)
	}
	doc, err = m.WebsiteConfig(site.ID)
	if err != nil || !strings.Contains(doc.Content, `add_header X-Config "value" always;`) {
		t.Fatalf("backup lost typed configuration: %v", err)
	}
}
