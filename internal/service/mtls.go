package service

import (
	"fmt"
	"slices"
	"time"
	"web-panel/internal/domain"
	"web-panel/internal/pki"
)

func (m *Manager) IssueMTLS(rev uint64, req pki.IssueRequest) (domain.Certificate, error) {
	var result domain.Certificate
	if req.Kind != "ca" && req.Kind != "client" {
		return result, fmt.Errorf("mTLS 仅签发 CA 或客户端证书")
	}
	err := m.mutate(rev, func(s *domain.State) error {
		var ca *domain.Certificate
		for i := range s.MTLS.Authorities {
			if s.MTLS.Authorities[i].ID == req.CAID {
				ca = &s.MTLS.Authorities[i]
			}
		}
		var e error
		result, e = pki.Issue(req, ca)
		if e != nil {
			return e
		}
		if req.Kind == "ca" {
			s.MTLS.Authorities = append(s.MTLS.Authorities, result)
		} else {
			s.MTLS.Clients = append(s.MTLS.Clients, result)
		}
		return nil
	})
	result.KeyPEM = ""
	return result, err
}
func (m *Manager) RevokeMTLS(rev uint64, id string) error {
	return m.mutate(rev, func(s *domain.State) error {
		i := slices.IndexFunc(s.MTLS.Clients, func(c domain.Certificate) bool { return c.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		if s.MTLS.Clients[i].RevokedAt != nil {
			return fmt.Errorf("证书已吊销")
		}
		now := time.Now().UTC()
		s.MTLS.Clients[i].RevokedAt = &now
		return nil
	})
}
func (m *Manager) DeleteMTLSCA(rev uint64, id string) error {
	return m.mutate(rev, func(s *domain.State) error {
		if slices.ContainsFunc(s.MTLS.Clients, func(c domain.Certificate) bool { return c.CAID == id && c.RevokedAt == nil }) {
			return fmt.Errorf("请先吊销此 CA 的有效客户端")
		}
		if slices.ContainsFunc(s.MTLS.Bindings, func(b domain.MTLSBinding) bool { return b.CAID == id }) {
			return fmt.Errorf("请先解除此 CA 的网站绑定")
		}
		if !slices.ContainsFunc(s.MTLS.Authorities, func(c domain.Certificate) bool { return c.ID == id }) {
			return ErrNotFound
		}
		s.MTLS.CAs = slices.DeleteFunc(s.MTLS.CAs, func(r domain.SSLCARecord) bool { return r.MaterialID == id })
		s.MTLS.Authorities = slices.DeleteFunc(s.MTLS.Authorities, func(c domain.Certificate) bool { return c.ID == id })
		s.MTLS.Clients = slices.DeleteFunc(s.MTLS.Clients, func(c domain.Certificate) bool { return c.CAID == id })
		return nil
	})
}
func (m *Manager) SaveMTLSBinding(rev uint64, b domain.MTLSBinding) error {
	return m.mutate(rev, func(s *domain.State) error {
		i := slices.IndexFunc(s.MTLS.Bindings, func(v domain.MTLSBinding) bool { return v.SiteID == b.SiteID })
		if b.CAID == "" {
			s.MTLS.Bindings = slices.DeleteFunc(s.MTLS.Bindings, func(v domain.MTLSBinding) bool { return v.SiteID == b.SiteID })
		} else if i < 0 {
			s.MTLS.Bindings = append(s.MTLS.Bindings, b)
		} else {
			s.MTLS.Bindings[i] = b
		}
		return nil
	})
}
func (m *Manager) MTLSBundle(id string, key, crl bool) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.store.Load()
	if e != nil {
		return nil, e
	}
	for _, c := range append(slices.Clone(s.MTLS.Authorities), s.MTLS.Clients...) {
		if c.ID != id {
			continue
		}
		if crl {
			if c.Kind != "ca" {
				return nil, fmt.Errorf("需要 mTLS CA")
			}
			return pki.CRL(c, s.MTLS.Clients)
		}
		text := c.CertPEM
		if key {
			text += c.KeyPEM
		}
		for _, ca := range s.MTLS.Authorities {
			if c.CAID == ca.ID {
				text += ca.CertPEM
			}
		}
		return []byte(text), nil
	}
	return nil, ErrNotFound
}
