package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
)

func (m *Manager) ReadPages(site string) (errorpages.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.store.Load()
	if err != nil {
		return errorpages.Document{}, err
	}
	if site != "" && !slices.ContainsFunc(state.Sites, func(s domain.Site) bool { return s.ID == site }) {
		return errorpages.Document{}, ErrNotFound
	}
	return errorpages.New(m.store.Root).Read(site)
}
func (m *Manager) SavePages(revision uint64, pages map[string]string) error {
	return m.savePageFiles(revision, "", "", pages)
}
func (m *Manager) UpdatePages(revision uint64, site string, value errorpages.Document) error {
	if value.ETag == "" {
		return fmt.Errorf("页面文件缺少内容版本，请重新加载")
	}
	return m.savePageFiles(revision, site, value.ETag, value.Pages)
}
func (m *Manager) savePageFiles(revision uint64, site, etag string, pages map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.store.Load()
	if err != nil {
		return err
	}
	if state.Revision != revision {
		return ErrConflict
	}
	if site != "" && !slices.ContainsFunc(state.Sites, func(s domain.Site) bool { return s.ID == site }) {
		return ErrNotFound
	}
	if err = errorpages.New(m.store.Root).Replace(site, etag, pages); err != nil {
		if errors.Is(err, errorpages.ErrConflict) {
			return errors.Join(ErrConflict, err)
		}
		return err
	}
	// Page files are independent of configuration revisions and nginx reloads.
	if runtime, ok := m.runtime.(interface{ SyncPages(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err = runtime.SyncPages(ctx); err != nil {
			return fmt.Errorf("页面文件已保存，容器同步失败，请重试保存或应用配置: %w", err)
		}
	}
	return nil
}
