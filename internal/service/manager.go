// Package service coordinates domain changes. Manager is the single writer for a data directory.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/store"
)

var ErrNotFound = errors.New("资源不存在")
var ErrConflict = store.ErrConflict

type Manager struct {
	jobContext context.Context
	mu         sync.Mutex
	store      *store.Store
	runtime    openresty.Runtime
}
type Overview struct {
	State   domain.State     `json:"state"`
	Runtime openresty.Status `json:"runtime"`
}

func New(s *store.Store, runtime openresty.Runtime) (*Manager, error) {
	m := &Manager{store: s, runtime: runtime, jobContext: context.Background()}
	if recovery, ok := runtime.(interface{ Recover(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := recovery.Recover(ctx); err != nil {
			return nil, err
		}
	}
	if err := m.recoverRestore(); err != nil {
		return nil, err
	}
	if _, err := s.Load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Overview(ctx context.Context) (Overview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return Overview{}, err
	}
	for i := range s.Certificates {
		s.Certificates[i].KeyPEM = ""
	}
	for i := range s.MTLS.Authorities {
		s.MTLS.Authorities[i].KeyPEM = ""
	}
	for i := range s.MTLS.Clients {
		s.MTLS.Clients[i].KeyPEM = ""
	}
	for i := range s.SSL.Records {
		s.SSL.Records[i].PrivateKey = ""
		s.SSL.Records[i].AcmeAccount.PrivateKey = ""
		s.SSL.Records[i].AcmeAccount.EabHmacKey = ""
	}
	for i := range s.SSL.CAs {
		s.SSL.CAs[i].PrivateKey = ""
	}
	for i := range s.MTLS.CAs {
		s.MTLS.CAs[i].PrivateKey = ""
	}
	for i := range s.SSL.Accounts {
		s.SSL.Accounts[i].Secret = ""
		s.SSL.Accounts[i].EabHmacKey = ""
	}
	for i := range s.SSL.DNSAccounts {
		s.SSL.DNSAccounts[i].Secret = ""
	}
	s.S3.SecretKey = ""
	return Overview{State: s, Runtime: m.runtime.Status(ctx)}, nil
}

func (m *Manager) mutate(revision uint64, fn func(*domain.State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	if revision != s.Revision {
		return ErrConflict
	}
	if err = fn(&s); err != nil {
		return err
	}
	domain.NormalizeState(&s)
	if err = domain.Validate(s); err != nil {
		return err
	}
	if err = m.validateNetwork(s); err != nil {
		return err
	}
	s.Revision++
	return m.store.Commit(revision, s)
}

func (m *Manager) SaveSite(revision uint64, site domain.Site) (domain.Site, error) {
	if site.ClientCAID != "" {
		return site, fmt.Errorf("mTLS 绑定请使用独立 mTLS 模块")
	}
	if site.LuaBindings == nil {
		site.LuaBindings = map[string]string{}
	}
	err := m.mutate(revision, func(s *domain.State) error {
		if site.ID == "" {
			site.ID = domain.NewID()
			site.PanelID = s.NextSiteID
			s.NextSiteID++
			site.CreatedAt = time.Now().UTC()
			normalizeWebsite(&site)
			s.Sites = append(s.Sites, site)
		} else {
			i := slices.IndexFunc(s.Sites, func(v domain.Site) bool { return v.ID == site.ID })
			if i < 0 {
				return ErrNotFound
			}
			site.PanelID = s.Sites[i].PanelID
			site.CreatedAt = s.Sites[i].CreatedAt
			normalizeWebsite(&site)
			s.Sites[i] = site
		}
		if err := domain.Validate(*s); err != nil {
			return err
		}
		dir := filepath.Join(m.store.Root, "sites", site.ID)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := m.checkRunDirectory(site.ID, site.RunDirectory); err != nil {
			return err
		}
		file, err := os.OpenFile(filepath.Join(dir, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = file.WriteString("<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>Welcome</title><body style=\"font-family:system-ui;max-width:640px;margin:15vh auto;padding:24px\"><h1>It works.</h1><p>你的网站已准备就绪。</p><small>OpenResty · WebCP</small></body></html>")
		return errors.Join(err, file.Close())
	})
	return site, err
}
func (m *Manager) DeleteSite(revision uint64, id string) error {
	return m.mutate(revision, func(s *domain.State) error {
		i := slices.IndexFunc(s.Sites, func(v domain.Site) bool { return v.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		s.Sites = slices.Delete(s.Sites, i, i+1)
		s.MTLS.Bindings = slices.DeleteFunc(s.MTLS.Bindings, func(b domain.MTLSBinding) bool { return b.SiteID == id })
		return nil
	})
}
func (m *Manager) SaveLua(revision uint64, l domain.Lua) (domain.Lua, error) {
	err := m.mutate(revision, func(s *domain.State) error {
		if l.ID == "" {
			l.ID = domain.NewID()
			s.Lua = append(s.Lua, l)
			return nil
		}
		i := slices.IndexFunc(s.Lua, func(v domain.Lua) bool { return v.ID == l.ID })
		if i < 0 {
			return ErrNotFound
		}
		s.Lua[i] = l
		return nil
	})
	return l, err
}
func (m *Manager) DeleteLua(revision uint64, id string) error {
	return m.mutate(revision, func(s *domain.State) error {
		i := slices.IndexFunc(s.Lua, func(v domain.Lua) bool { return v.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		s.Lua = slices.Delete(s.Lua, i, i+1)
		return nil
	})
}
func (m *Manager) SaveS3(revision uint64, config domain.S3) error {
	config.Prefix = strings.Trim(config.Prefix, "/")
	if config.Prefix != "" {
		config.Prefix = path.Clean(config.Prefix)
	}
	return m.mutate(revision, func(s *domain.State) error {
		if config.SecretKey == "" {
			config.SecretKey = s.S3.SecretKey
		}
		s.S3 = config
		return nil
	})
}

func (m *Manager) Preview() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return "", err
	}
	g, err := m.render(s)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(g.Path)
	return g.Config, nil
}
func (m *Manager) Apply(ctx context.Context, revision uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	if revision != s.Revision {
		return ErrConflict
	}
	g, err := m.render(s)
	if err != nil {
		return err
	}
	if err = m.runtime.Apply(ctx, g); err != nil {
		// Retain the generation after a runtime failure. A lost Docker connection
		// can make the reload result uncertain; a worker may still reference it.
		return err
	}
	return nil
}
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtime.Stop(ctx)
}
func (m *Manager) Logs(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtime.Logs(ctx)
}

// marshal is used for durable restore journals, never HTTP responses.
func marshal(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
