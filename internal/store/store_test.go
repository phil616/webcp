package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
)

func TestLegacyStateMigratesToSQLiteAndEngineFiles(t *testing.T) {
	root := t.TempDir()
	state := domain.Default()
	state.Schema = 1
	state.Revision = 19
	data, _ := json.Marshal(state)
	var legacy map[string]any
	json.Unmarshal(data, &legacy)
	delete(legacy, "streams")
	legacy["pages"] = map[string]string{"404": "legacy custom HTML"}
	data, _ = json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(root, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	current, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if current.Streams == nil || current.Schema != domain.SchemaVersion || current.Revision != 19 {
		t.Fatal("legacy configuration changed", current)
	}
	pages, err := errorpages.New(root).Read("")
	if err != nil || pages.Pages["404"] != "legacy custom HTML" {
		t.Fatal("legacy page missing", err)
	}
	if _, err = os.Stat(filepath.Join(root, "state.json")); !os.IsNotExist(err) {
		t.Fatal("JSON is still an active store")
	}
	if original, err := os.ReadFile(filepath.Join(root, "state.json.migrated")); err != nil || string(original) != string(data) {
		t.Fatal("original JSON was not preserved")
	}
	bytes, err := os.ReadFile(s.Path())
	if err != nil || string(bytes[:16]) != "SQLite format 3\x00" {
		t.Fatal("not a SQLite database", err)
	}
	var mode string
	if err = s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatal("WAL disabled", err)
	}
	var sync int
	if err = s.db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil || sync != 2 {
		t.Fatal("durable commits disabled", err)
	}
}
func TestSQLiteAtomicCASAndRestart(t *testing.T) {
	root := t.TempDir()
	first, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	state, _ := first.Load()
	state.Revision++
	state.Lua = []domain.Lua{{ID: domain.NewID(), Name: "first", Kind: "module", Code: "return {}"}}
	if err = first.Commit(1, state); err != nil {
		t.Fatal(err)
	}
	state.Lua[0].Code = "return {stale=true}"
	if err = second.Commit(1, state); !errors.Is(err, ErrConflict) {
		t.Fatal("stale writer committed", err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := second.Load()
	if err != nil || loaded.Revision != 2 || loaded.Lua[0].Code != "return {}" {
		t.Fatal("committed state lost", err)
	}
	// A database constraint failure midway through a write must roll back metadata and all resources.
	invalid := loaded
	invalid.Revision++
	invalid.SSL.Accounts = []domain.SSLAccount{{}, {}}
	if err = second.Commit(2, invalid); err == nil {
		t.Fatal("duplicate SQL primary keys accepted")
	}
	loaded, err = second.Load()
	if err != nil || loaded.Revision != 2 || len(loaded.SSL.Accounts) != 0 {
		t.Fatal("partial transaction committed", err)
	}
}
func TestCorruptLegacyDoesNotInitializeDefaults(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "state.json")
	os.WriteFile(file, []byte("{broken"), 0600)
	if _, err := Open(root); err == nil {
		t.Fatal("corrupt legacy configuration accepted")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "{broken" {
		t.Fatal("legacy source changed")
	}
}

func TestWALReadersSeeCommittedStateDuringWrite(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE app_state SET revision=2"); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil || state.Revision != 1 {
		t.Fatal("reader saw an uncommitted write", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	state, err = s.Load()
	if err != nil || state.Revision != 2 {
		t.Fatal("reader missed committed write", err)
	}
}

func TestCommitRequiresNextRevision(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.Revision = 10
	if err = s.Commit(1, state); err == nil {
		t.Fatal("nonsequential revision accepted")
	}
	current, err := s.Load()
	if err != nil || current.Revision != 1 {
		t.Fatal("invalid commit mutated state", err)
	}
}
