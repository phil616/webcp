package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Machine-local runtime selection is intentionally excluded from portable exports.
func (s *Store) LoadConnection() ([]byte, error) {
	var data []byte
	err := s.db.QueryRow("SELECT document FROM runtime_connection WHERE id=1").Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}
func (s *Store) SaveConnection(expected uint64, data []byte) error {
	var value struct {
		Version uint64 `json:"version"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version uint64
	err = tx.QueryRow("SELECT version FROM runtime_connection WHERE id=1").Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && version != expected {
		return ErrConflict
	}
	_, err = tx.Exec("INSERT INTO runtime_connection VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET version=excluded.version,document=excluded.document", value.Version, string(data))
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) SaveBackup(id, created string, data []byte) error {
	_, err := s.db.Exec("INSERT INTO backup_records VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET created_at=excluded.created_at,document=excluded.document", id, created, string(data))
	return err
}
func (s *Store) BackupMetadata(id string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow("SELECT document FROM backup_records WHERE id=?", id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}
	return data, err
}
func (s *Store) ListBackupMetadata() ([][]byte, error) {
	rows, err := s.db.Query("SELECT document FROM backup_records ORDER BY created_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := [][]byte{}
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		result = append(result, data)
	}
	return result, rows.Err()
}
func (s *Store) RestoreJournal() ([]byte, error) {
	var data []byte
	err := s.db.QueryRow("SELECT document FROM restore_operations WHERE id=1").Scan(&data)
	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}
	return data, err
}
func (s *Store) SaveRestoreJournal(data []byte) error {
	_, err := s.db.Exec("INSERT INTO restore_operations VALUES(1,?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", string(data))
	return err
}
func (s *Store) ClearRestoreJournal() error {
	_, err := s.db.Exec("DELETE FROM restore_operations WHERE id=1")
	return err
}
func (s *Store) importOperationalMetadata() error {
	connection := filepath.Join(s.Root, "runtime", "connection.json")
	if data, err := os.ReadFile(connection); err == nil {
		current, e := s.LoadConnection()
		if e != nil {
			return e
		}
		if len(current) == 0 {
			err = s.SaveConnection(0, data)
		}
		if err != nil {
			return err
		}
		if err = archiveLegacy(connection); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(s.Root, "backups", "*.json"))
	if err != nil {
		return err
	}
	for _, file := range paths {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var value struct {
			ID      string `json:"id"`
			Created string `json:"createdAt"`
		}
		if err = json.Unmarshal(data, &value); err != nil {
			return err
		}
		if _, err = s.db.Exec("INSERT INTO backup_records VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", value.ID, value.Created, string(data)); err != nil {
			return err
		}
		if err = archiveLegacy(file); err != nil {
			return err
		}
	}
	return nil
}

// archiveLegacy is restart-safe and never replaces a different migration backup.
func archiveLegacy(file string) error {
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	target := file + ".migrated"
	previous, err := os.ReadFile(target)
	if err == nil {
		if !bytes.Equal(previous, content) {
			return fmt.Errorf("旧配置备份已存在且内容不同，拒绝覆盖: %s", target)
		}
		return os.Remove(file)
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err = os.Chmod(file, 0600); err != nil {
		return err
	}
	return os.Rename(file, target)
}
