// Package store owns SQLite migrations and transactional controller repositories.
// Engine HTML is a filesystem resource and has no representation in this schema.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
	"web-panel/internal/fsutil"
)

var ErrConflict = errors.New("配置版本已变化，请刷新后重试")

type Store struct {
	Root string
	db   *sql.DB
	read *sql.DB
}

//go:embed migrations/001_initial.sql
var initialSchema string

func Open(root string) (_ *Store, err error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{"sites", "runtime/generations", "runtime/sites", "runtime/cache", "runtime/acme", "ssl/logs", "cache", "backups"} {
		if err = os.MkdirAll(filepath.Join(absolute, dir), 0700); err != nil {
			return nil, err
		}
	}
	s := &Store{Root: absolute}
	// Create with private permissions before SQLite can create the database or its WAL.
	file, err := os.OpenFile(s.Path(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	if err = os.Chmod(s.Path(), 0600); err != nil {
		return nil, err
	}
	location := url.URL{Scheme: "file", Path: s.Path()}
	query := url.Values{}
	for _, pragma := range []string{"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)", "foreign_keys(ON)"} {
		query.Add("_pragma", pragma)
	}
	query.Set("_txlock", "immediate")
	location.RawQuery = query.Encode()
	s.db, err = sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	// A single writer connection makes transactions predictable; WAL still permits
	// independent operational readers and SQLite provides cross-process serialization.
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	defer func() {
		if err != nil {
			s.db.Close()
		}
	}()
	if err = s.migrate(); err != nil {
		return nil, err
	}
	if err = s.initialize(); err != nil {
		return nil, err
	}
	readLocation := url.URL{Scheme: "file", Path: s.Path()}
	readQuery := url.Values{}
	readQuery.Set("mode", "ro")
	readQuery.Add("_pragma", "busy_timeout(5000)")
	readQuery.Add("_pragma", "query_only(ON)")
	readLocation.RawQuery = readQuery.Encode()
	s.read, err = sql.Open("sqlite", readLocation.String())
	if err != nil {
		return nil, err
	}
	s.read.SetMaxOpenConns(4)
	s.read.SetMaxIdleConns(4)
	return s, nil
}
func (s *Store) Path() string { return filepath.Join(s.Root, "webcp.db") }
func (s *Store) Close() error {
	var readErr error
	if s.read != nil {
		readErr = s.read.Close()
	}
	return errors.Join(readErr, s.db.Close())
}
func (s *Store) migrate() error {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("SQLite 数据库版本 %d 高于此程序支持版本", version)
	}
	if version == 0 {
		if _, err = tx.Exec(initialSchema); err != nil {
			return err
		}
		if _, err = tx.Exec("PRAGMA user_version=1"); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) initialize() error {
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM app_state").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		if err := errorpages.New(s.Root).Ensure(nil); err != nil {
			return err
		}
		if err := archiveLegacy(filepath.Join(s.Root, "state.json")); err != nil {
			return err
		}
		return s.importOperationalMetadata()
	}
	state := domain.Default()
	pages := map[string]map[string]string{}
	legacy := filepath.Join(s.Root, "state.json")
	data, err := os.ReadFile(legacy)
	if err == nil {
		state, pages, err = domain.DecodeExport(data)
		if err != nil {
			return fmt.Errorf("迁移旧配置失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Import files first, then commit the database. Retrying after interruption only
	// initializes missing files and never overwrites a filesystem edit.
	if err = errorpages.New(s.Root).Ensure(pages); err != nil {
		return err
	}
	if err = s.Save(state); err != nil {
		return err
	}
	if len(data) > 0 {
		if err = archiveLegacy(legacy); err != nil {
			return err
		}
	}
	return s.importOperationalMetadata()
}
func (s *Store) Load() (domain.State, error) {
	tx, err := s.read.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.State{}, err
	}
	defer tx.Rollback()
	state, err := loadState(tx)
	if err != nil {
		return state, err
	}
	return state, tx.Commit()
}
func (s *Store) Save(state domain.State) error { return s.save(nil, state) }
func (s *Store) Commit(expected uint64, state domain.State) error {
	if expected == 0 || state.Revision != expected+1 {
		return fmt.Errorf("提交配置版本必须严格递增")
	}
	return s.save(&expected, state)
}
func (s *Store) save(expected *uint64, state domain.State) error {
	domain.NormalizeState(&state)
	if err := domain.Validate(state); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(data) > domain.MaxStateSize {
		return fmt.Errorf("配置快照超过 16 MiB 限制")
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if expected != nil {
		var revision uint64
		if err = tx.QueryRow("SELECT revision FROM app_state WHERE id=1").Scan(&revision); err != nil {
			return err
		}
		if revision != *expected {
			return ErrConflict
		}
	}
	if err = saveState(tx, state); err != nil {
		return err
	}
	return tx.Commit()
}
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	return fsutil.AtomicWrite(path, data, mode)
}
