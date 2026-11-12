package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"web-panel/internal/backup"
	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
)

func (m *Manager) CheckS3(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	client, err := backup.NewS3(s.S3)
	if err != nil {
		return err
	}
	return client.Check(ctx)
}
func (m *Manager) Backup(ctx context.Context, revision uint64, remote bool) (backup.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var record backup.Record
	s, err := m.store.Load()
	if err != nil {
		return record, err
	}
	if revision != s.Revision {
		return record, ErrConflict
	}
	var client *backup.S3
	if remote {
		client, err = backup.NewS3(s.S3)
		if err != nil {
			return record, err
		}
	}
	record, err = backup.Create(m.store.Root, s)
	if err != nil {
		return record, err
	}
	save := func() error {
		b, e := marshal(record)
		if e != nil {
			return e
		}
		return m.store.SaveBackup(record.ID, record.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), b)
	}
	if err = save(); err != nil {
		return record, err
	}
	if remote {
		record.ObjectKey, err = client.Upload(ctx, filepath.Join(m.store.Root, "backups", record.ID+".tar.gz"), record)
		if err != nil {
			return record, fmt.Errorf("本地备份已保留，但 S3 上传失败: %w", err)
		}
		record.Remote = true
		err = save()
	}
	return record, err
}
func (m *Manager) Backups(ctx context.Context, remote bool) ([]backup.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []backup.Record{}
	if remote {
		s, err := m.store.Load()
		if err != nil {
			return nil, err
		}
		client, err := backup.NewS3(s.S3)
		if err != nil {
			return nil, err
		}
		items, err = client.List(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		documents, err := m.store.ListBackupMetadata()
		if err != nil {
			return nil, err
		}
		for _, data := range documents {
			var r backup.Record
			if err = json.Unmarshal(data, &r); err != nil {
				return nil, err
			}
			items = append(items, r)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
func (m *Manager) OpenBackup(id string) (*os.File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !domain.ValidID(id) {
		return nil, ErrNotFound
	}
	return os.Open(filepath.Join(m.store.Root, "backups", id+".tar.gz"))
}

type restoreJournal struct {
	Stage    string       `json:"stage"`
	OldSites string       `json:"oldSites"`
	OldPages string       `json:"oldPages,omitempty"`
	OldState domain.State `json:"oldState"`
}

// A surviving journal means the directory swap did not commit. Roll back on startup.
func (m *Manager) recoverRestore() error {
	journalPath := filepath.Join(m.store.Root, "restore-journal.json")
	b, err := m.store.RestoreJournal()
	legacyJournal := false
	if os.IsNotExist(err) {
		b, err = os.ReadFile(journalPath)
		legacyJournal = err == nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var j restoreJournal
	if err = json.Unmarshal(b, &j); err != nil {
		return err
	}
	legacyPages := map[string]map[string]string{}
	if legacyJournal {
		var old struct {
			State json.RawMessage `json:"oldState"`
		}
		if err = json.Unmarshal(b, &old); err != nil {
			return err
		}
		j.OldState, legacyPages, err = domain.DecodeExport(old.State)
		if err != nil {
			return err
		}
	} else if err = domain.Validate(j.OldState); err != nil {
		return err
	}
	if !domain.ValidID(j.Stage) || !domain.ValidID(j.OldSites) {
		return fmt.Errorf("无效恢复日志")
	}
	if j.OldPages != "" && !domain.ValidID(j.OldPages) {
		return fmt.Errorf("无效页面恢复日志")
	}
	old := filepath.Join(m.store.Root, "restore-old-"+j.OldSites)
	sites := filepath.Join(m.store.Root, "sites")
	if _, err = os.Stat(old); err == nil {
		if err = os.RemoveAll(sites); err != nil {
			return err
		}
		if err = os.Rename(old, sites); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if j.OldPages != "" {
		previous := filepath.Join(m.store.Root, "restore-pages-old-"+j.OldPages)
		if _, err = os.Stat(previous); err == nil {
			if err = os.RemoveAll(errorpages.New(m.store.Root).Root); err != nil {
				return err
			}
			if err = os.Rename(previous, errorpages.New(m.store.Root).Root); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for scope, html := range legacyPages {
		if err = errorpages.New(m.store.Root).Replace(scope, "", html); err != nil {
			return err
		}
	}
	if err = m.store.Save(j.OldState); err != nil {
		return err
	}
	if legacyJournal {
		err = os.Remove(journalPath)
	} else {
		err = m.store.ClearRestoreJournal()
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(m.store.Root, "restore-stage-"+j.Stage))
}

// Restore deliberately stops OpenResty before replacing website content. Restored config remains a draft.
func (m *Manager) Restore(ctx context.Context, revision uint64, id string, remote bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !domain.ValidID(id) {
		return ErrNotFound
	}
	state, err := m.store.Load()
	if err != nil {
		return err
	}
	if state.Revision != revision {
		return ErrConflict
	}
	stageID := domain.NewID()
	stage := filepath.Join(m.store.Root, "restore-stage-"+stageID)
	if err = os.Mkdir(stage, 0700); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var reader io.ReadCloser
	var expected string
	if remote {
		client, e := backup.NewS3(state.S3)
		if e != nil {
			return e
		}
		records, e := client.List(ctx)
		if e != nil {
			return e
		}
		for _, r := range records {
			if r.ID == id {
				expected = r.SHA256
				reader, err = client.Download(ctx, r.ObjectKey)
				break
			}
		}
		if err != nil {
			return err
		}
		if reader == nil {
			return ErrNotFound
		}
	} else {
		b, e := m.store.BackupMetadata(id)
		if e != nil {
			return e
		}
		var r backup.Record
		if e = json.Unmarshal(b, &r); e != nil {
			return e
		}
		expected = r.SHA256
		reader, err = os.Open(filepath.Join(m.store.Root, "backups", id+".tar.gz"))
		if err != nil {
			return err
		}
	}
	defer reader.Close()
	if len(expected) != 64 {
		return fmt.Errorf("备份缺少 SHA-256 元数据，拒绝恢复")
	}
	archive, err := os.OpenFile(filepath.Join(stage, "archive.tar.gz"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer archive.Close()
	n, err := io.Copy(archive, io.LimitReader(reader, backup.MaxArchiveSize+1))
	if err != nil {
		return err
	}
	if n > backup.MaxArchiveSize {
		return fmt.Errorf("备份文件过大")
	}
	if _, err = archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = backup.Verify(archive, expected); err != nil {
		return err
	}
	if _, err = archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	restored, err := backup.Extract(archive, stage)
	if err != nil {
		return err
	}
	// Ensure every registered website has a root, including empty proxy roots.
	for _, site := range restored.Sites {
		if err = os.MkdirAll(filepath.Join(stage, "sites", site.ID), 0700); err != nil {
			return err
		}
	}
	if err = m.runtime.Stop(ctx); err != nil {
		return fmt.Errorf("恢复前停止 OpenResty 失败: %w", err)
	}
	j := restoreJournal{Stage: stageID, OldSites: domain.NewID(), OldPages: domain.NewID(), OldState: state}
	b, err := marshal(j)
	if err != nil {
		return err
	}
	if err = m.store.SaveRestoreJournal(b); err != nil {
		return err
	}
	rollback := func(cause error) error {
		if e := m.recoverRestore(); e != nil {
			return fmt.Errorf("恢复失败 (%v)，回滚失败: %w", cause, e)
		}
		return cause
	}
	if err = os.Rename(filepath.Join(m.store.Root, "sites"), filepath.Join(m.store.Root, "restore-old-"+j.OldSites)); err != nil {
		return rollback(err)
	}
	if err = os.Rename(filepath.Join(stage, "sites"), filepath.Join(m.store.Root, "sites")); err != nil {
		return rollback(err)
	}
	if err = os.Rename(errorpages.New(m.store.Root).Root, filepath.Join(m.store.Root, "restore-pages-old-"+j.OldPages)); err != nil {
		return rollback(err)
	}
	if err = os.Rename(errorpages.New(stage).Root, errorpages.New(m.store.Root).Root); err != nil {
		return rollback(err)
	}
	restored.Revision = state.Revision + 1
	if err = m.store.Commit(state.Revision, restored); err != nil {
		return rollback(err)
	}
	if err = m.store.ClearRestoreJournal(); err != nil {
		return rollback(err)
	}
	if err = os.RemoveAll(filepath.Join(m.store.Root, "restore-pages-old-"+j.OldPages)); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(m.store.Root, "restore-old-"+j.OldSites))
}
