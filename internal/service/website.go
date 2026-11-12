package service

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
)

type HTTPSUpdate struct {
	TLSID string             `json:"tlsId"`
	HTTPS domain.HTTPSConfig `json:"https"`
}

func (m *Manager) validateNetwork(s domain.State) error {
	if v, ok := m.runtime.(interface{ ValidateState(domain.State) error }); ok {
		return v.ValidateState(s)
	}
	return nil
}
func (m *Manager) render(s domain.State) (openresty.Generation, error) {
	if r, ok := m.runtime.(interface {
		RenderOptions() openresty.RenderOptions
	}); ok {
		return openresty.Render(m.store.Root, s, r.RenderOptions())
	}
	return openresty.Render(m.store.Root, s)
}
func normalizeProxy(p *domain.ProxyRule) {
	if p.ID == "" {
		p.ID = domain.NewID()
	}
	if p.ProxyHost == "" {
		p.ProxyHost = "$host"
	}
	if p.ConnectTimeout == 0 {
		p.ConnectTimeout = 60
	}
	if p.ReadTimeout == 0 {
		p.ReadTimeout = 60
	}
	if p.SendTimeout == 0 {
		p.SendTimeout = 60
	}
	if p.ServerCacheTime == 0 {
		p.ServerCacheTime = 10
		p.ServerCacheUnit = "m"
	}
	if p.Replaces == nil {
		p.Replaces = []domain.Replacement{}
	}
}
func normalizeWebsite(s *domain.Site) {
	domain.NormalizeSite(s)
	for i := range s.Proxies {
		normalizeProxy(&s.Proxies[i])
	}
	for i := range s.Redirects {
		if s.Redirects[i].ID == "" {
			s.Redirects[i].ID = domain.NewID()
		}
	}
}
func (m *Manager) updateWebsite(revision uint64, id string, change func(*domain.Site) error) error {
	return m.mutate(revision, func(s *domain.State) error {
		i := slices.IndexFunc(s.Sites, func(site domain.Site) bool { return site.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		if err := change(&s.Sites[i]); err != nil {
			return err
		}
		normalizeWebsite(&s.Sites[i])
		return nil
	})
}
func (m *Manager) SaveBindings(rev uint64, id string, bindings []domain.DomainBinding) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		s.Bindings = bindings
		if s.Bindings == nil {
			s.Bindings = []domain.DomainBinding{}
		}
		return nil
	})
}
func (m *Manager) SaveDirectory(rev uint64, id, dir string) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		if err := domain.ValidateRunDirectory(dir); err != nil {
			return err
		}
		if err := m.checkRunDirectory(id, dir); err != nil {
			return err
		}
		s.RunDirectory = dir
		return nil
	})
}
func (m *Manager) SaveIndex(rev uint64, id string, index []string) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		s.Index = index
		if s.Index == nil {
			s.Index = []string{}
		}
		return nil
	})
}
func (m *Manager) SaveHTTPS(rev uint64, id string, v HTTPSUpdate) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		s.TLSID = v.TLSID
		if s.TLSID == "" {
		}
		s.HTTPS = &v.HTTPS
		return nil
	})
}
func (m *Manager) SaveCORS(rev uint64, id string, v domain.CORSConfig) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error { s.CORS = v; return nil })
}
func (m *Manager) SaveRealIP(rev uint64, id string, v domain.RealIPConfig) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error { s.RealIP = v; return nil })
}
func (m *Manager) SaveProxy(rev uint64, id string, v domain.ProxyRule) (domain.ProxyRule, error) {
	err := m.updateWebsite(rev, id, func(s *domain.Site) error {
		if v.ID == "" {
			normalizeProxy(&v)
			s.Proxies = append(s.Proxies, v)
			return nil
		}
		i := slices.IndexFunc(s.Proxies, func(p domain.ProxyRule) bool { return p.ID == v.ID })
		if i < 0 {
			return ErrNotFound
		}
		if v.Name != s.Proxies[i].Name {
			return fmt.Errorf("代理规则名称创建后不能修改")
		}
		normalizeProxy(&v)
		s.Proxies[i] = v
		return nil
	})
	return v, err
}
func (m *Manager) DeleteProxy(rev uint64, id, rule string) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		i := slices.IndexFunc(s.Proxies, func(p domain.ProxyRule) bool { return p.ID == rule })
		if i < 0 {
			return ErrNotFound
		}
		s.Proxies = slices.Delete(s.Proxies, i, i+1)
		return nil
	})
}
func (m *Manager) SaveRedirect(rev uint64, id string, v domain.RedirectRule) (domain.RedirectRule, error) {
	err := m.updateWebsite(rev, id, func(s *domain.Site) error {
		if v.ID == "" {
			v.ID = domain.NewID()
			s.Redirects = append(s.Redirects, v)
			return nil
		}
		i := slices.IndexFunc(s.Redirects, func(r domain.RedirectRule) bool { return r.ID == v.ID })
		if i < 0 {
			return ErrNotFound
		}
		if v.Name != s.Redirects[i].Name || v.Type != s.Redirects[i].Type {
			return fmt.Errorf("重定向名称和类型创建后不能修改")
		}
		s.Redirects[i] = v
		return nil
	})
	return v, err
}
func (m *Manager) DeleteRedirect(rev uint64, id, rule string) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		i := slices.IndexFunc(s.Redirects, func(r domain.RedirectRule) bool { return r.ID == rule })
		if i < 0 {
			return ErrNotFound
		}
		s.Redirects = slices.Delete(s.Redirects, i, i+1)
		return nil
	})
}
func (m *Manager) checkRunDirectory(id, dir string) error {
	root, err := os.OpenRoot(filepath.Join(m.store.Root, "sites", id))
	if err != nil {
		return err
	}
	defer root.Close()
	relative := strings.TrimPrefix(dir, "/")
	if relative == "" {
		relative = "."
	}
	current := ""
	for _, part := range strings.Split(relative, "/") {
		current = filepath.Join(current, part)
		info, e := root.Lstat(current)
		if e != nil {
			return fmt.Errorf("运行目录不存在: %w", e)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("运行目录必须是真实目录，不能是符号链接")
		}
	}
	return nil
}
func (m *Manager) Directories(id string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, err := m.siteRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dirs := []string{"/"}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if name == "." {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			dirs = append(dirs, "/"+name)
		}
		if len(dirs) > 10000 {
			return fmt.Errorf("目录数超过 10000")
		}
		return nil
	})
	return dirs, err
}
func (m *Manager) ClearProxyCache(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(s.Sites, func(site domain.Site) bool { return site.ID == id }) {
		return ErrNotFound
	}
	r, ok := m.runtime.(interface {
		ClearProxyCache(context.Context, string) error
	})
	if !ok {
		return fmt.Errorf("当前运行时不支持清理代理缓存")
	}
	return r.ClearProxyCache(ctx, id)
}

func (m *Manager) SaveProxyCache(rev uint64, id string, v domain.ProxyCacheConfig) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error { s.ProxyCache = &v; return nil })
}
func (m *Manager) RulePreview(id, rule, kind string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return "", err
	}
	for _, site := range s.Sites {
		if site.ID != id {
			continue
		}
		if kind == "proxies" {
			for _, p := range site.Proxies {
				if p.ID == rule {
					return openresty.ProxyFragment(site, p), nil
				}
			}
		} else {
			for _, r := range site.Redirects {
				if r.ID == rule {
					return openresty.RedirectFragment(r), nil
				}
			}
		}
	}
	return "", ErrNotFound
}

type ExtensionUpdate struct {
	LuaBindings *map[string]string `json:"luaBindings"`
}

func (m *Manager) SaveExtensions(rev uint64, id string, v ExtensionUpdate) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error {
		if v.LuaBindings == nil {
			return fmt.Errorf("请提供 Lua 绑定")
		}
		s.LuaBindings = *v.LuaBindings
		return nil
	})
}

func (m *Manager) SaveWebsiteName(rev uint64, id, name string) error {
	return m.updateWebsite(rev, id, func(s *domain.Site) error { s.Name = name; return nil })
}

func (m *Manager) HTTPSPort() int {
	if r, ok := m.runtime.(interface {
		RenderOptions() openresty.RenderOptions
	}); ok {
		if port := r.RenderOptions().HTTPSPort; port > 0 {
			return port
		}
	}
	return 443
}
