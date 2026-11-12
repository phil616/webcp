package errorpages

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEngineFilesETagAndExternalEdits(t *testing.T) {
	f := New(t.TempDir())
	if err := f.Ensure(nil); err != nil {
		t.Fatal(err)
	}
	first, err := f.Read("")
	if err != nil || len(first.Pages) != 8 {
		t.Fatal("default assets missing", err)
	}
	os.WriteFile(filepath.Join(f.Root, "default/404.html"), []byte("external filesystem edit"), 0600)
	second, err := f.Read("")
	if err != nil || second.Pages["404"] != "external filesystem edit" || first.ETag == second.ETag {
		t.Fatal("file not authoritative", err)
	}
	if err = f.Replace("", first.ETag, first.Pages); !errors.Is(err, ErrConflict) {
		t.Fatal("stale editor overwrote files", err)
	}
	second.Pages["404"] = "editor update"
	if err = f.Replace("", second.ETag, second.Pages); err != nil {
		t.Fatal(err)
	}
	third, _ := f.Read("")
	if third.Pages["404"] != "editor update" {
		t.Fatal("file not updated")
	}
	if err = f.Ensure(nil); err != nil {
		t.Fatal(err)
	}
	fourth, _ := f.Read("")
	if fourth.ETag != third.ETag {
		t.Fatal("bootstrap overwrote operator edits")
	}
}
func TestPageFileValidation(t *testing.T) {
	f := New(t.TempDir())
	f.Ensure(nil)
	if err := f.Replace("../outside", "", map[string]string{"404": "bad"}); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := f.Replace("", "", map[string]string{"../bad": "bad"}); err == nil {
		t.Fatal("invalid status accepted")
	}
	external := filepath.Join(t.TempDir(), "outside.html")
	os.WriteFile(external, []byte("outside"), 0600)
	target := filepath.Join(f.Root, "default/404.html")
	os.Remove(target)
	os.Symlink(external, target)
	if _, err := f.Read(""); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestBootstrapPreservesIntentionallyMissingPages(t *testing.T) {
	f := New(t.TempDir())
	if err := f.Ensure(nil); err != nil {
		t.Fatal(err)
	}
	first, _ := f.Read("")
	if err := f.Replace("", first.ETag, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := f.Ensure(nil); err != nil {
		t.Fatal(err)
	}
	current, _ := f.Read("")
	if len(current.Pages) != 0 {
		t.Fatal("bootstrap recreated intentionally removed pages")
	}
}

func TestEmptyExternalPageCanBeRepairedWithoutBlockingStartup(t *testing.T) {
	files := New(t.TempDir())
	if err := files.Ensure(nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(files.Root, "default/404.html"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := files.Ensure(nil); err != nil {
		t.Fatal("engine content blocked controller startup", err)
	}
	document, err := files.Read("")
	if err != nil || document.Pages["404"] != "" {
		t.Fatal("empty file cannot be opened for repair", err)
	}
	document.Pages["404"] = "repaired"
	if err = files.Replace("", document.ETag, document.Pages); err != nil {
		t.Fatal(err)
	}
}
