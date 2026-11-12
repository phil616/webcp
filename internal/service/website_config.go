package service

import (
	"fmt"
	"slices"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
)

type WebsiteConfigDocument struct {
	Content  string `json:"content"`
	Revision uint64 `json:"revision"`
}

func (m *Manager) WebsiteConfig(id string) (WebsiteConfigDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.store.Load()
	if err != nil {
		return WebsiteConfigDocument{}, err
	}
	i := slices.IndexFunc(s.Sites, func(site domain.Site) bool { return site.ID == id })
	if i < 0 {
		return WebsiteConfigDocument{}, ErrNotFound
	}
	site := s.Sites[i]
	site.ClientCAID = domain.MTLSAuthority(s, id)
	text, err := openresty.WebsiteConfig(site, m.HTTPSPort())
	return WebsiteConfigDocument{Content: text, Revision: s.Revision}, err
}

func (m *Manager) SaveWebsiteConfig(revision uint64, id, text string) error {
	return m.mutate(revision, func(state *domain.State) error {
		i := slices.IndexFunc(state.Sites, func(site domain.Site) bool { return site.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		site := state.Sites[i]
		site.ClientCAID = domain.MTLSAuthority(*state, id)
		candidate, err := openresty.ImportWebsiteConfig(site, text, m.HTTPSPort())
		if err != nil {
			return fmt.Errorf("网站配置未保存: %w", err)
		}
		if err = m.checkRunDirectory(id, candidate.RunDirectory); err != nil {
			return err
		}
		// mTLS remains owned by the independent binding module.
		candidate.ClientCAID = ""
		state.Sites[i] = candidate
		return nil
	})
}

type WebsiteAdvancedUpdate struct {
	Headers  []domain.ResponseHeader `json:"headers"`
	Security domain.WebsiteSecurity  `json:"security"`
}

func (m *Manager) SaveWebsiteAdvanced(revision uint64, id string, value WebsiteAdvancedUpdate) error {
	return m.updateWebsite(revision, id, func(site *domain.Site) error {
		site.Headers, site.Security = value.Headers, value.Security
		return nil
	})
}
