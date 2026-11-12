package domain

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"web-panel/internal/onepanel/model"
)

// SSL metadata uses the upstream schema. PEM material remains canonical in
// State.Certificates; list responses never contain private keys.
type SSLRecord struct {
	model.WebsiteSSL
	MaterialID   string `json:"materialId"`
	Validity     int    `json:"validity"`
	ValidityUnit string `json:"validityUnit"`
}
type SSLAccount struct {
	model.WebsiteAcmeAccount
	Secret string `json:"accountPrivateKey,omitempty"`
}
type SSLDNSAccount struct {
	model.WebsiteDnsAccount
	Secret string `json:"authorization,omitempty"`
}
type SSLState struct {
	NextID      uint            `json:"nextId"`
	Records     []SSLRecord     `json:"records"`
	Accounts    []SSLAccount    `json:"accounts"`
	DNSAccounts []SSLDNSAccount `json:"dnsAccounts"`
	CAs         []SSLCARecord   `json:"cas"`
}
type SSLCARecord struct {
	model.WebsiteCA
	MaterialID string `json:"materialId"`
}
type MTLSBinding struct {
	SiteID  string `json:"siteId"`
	CAID    string `json:"caId"`
	Enabled bool   `json:"enabled"`
}
type MTLSState struct {
	NextID      uint          `json:"nextId"`
	CAs         []SSLCARecord `json:"cas"`
	Authorities []Certificate `json:"authorities"`
	Clients     []Certificate `json:"clients"`
	Bindings    []MTLSBinding `json:"bindings"`
}

func MTLSAuthority(state State, siteID string) string {
	for _, b := range state.MTLS.Bindings {
		if b.SiteID == siteID && b.Enabled {
			return b.CAID
		}
	}
	return ""
}
func normalizeCertificates(s *State) {
	if s.MTLS.Authorities == nil {
		s.MTLS.Authorities = []Certificate{}
	}
	if s.MTLS.Clients == nil {
		s.MTLS.Clients = []Certificate{}
	}
	if s.MTLS.Bindings == nil {
		s.MTLS.Bindings = []MTLSBinding{}
	}
	// Migration copies an existing dual-use CA into the mTLS namespace. It never
	// reissues certificates, changes serials or restores a revoked credential.
	copyCA := func(id string) {
		for _, ca := range s.MTLS.Authorities {
			if ca.ID == id {
				return
			}
		}
		for _, ca := range s.Certificates {
			if ca.ID == id && ca.Kind == "ca" {
				s.MTLS.Authorities = append(s.MTLS.Authorities, ca)
				return
			}
		}
	}
	ssl := []Certificate{}
	for _, c := range s.Certificates {
		if c.Kind == "client" {
			copyCA(c.CAID)
			found := false
			for _, old := range s.MTLS.Clients {
				if old.ID == c.ID {
					found = true
				}
			}
			if !found {
				s.MTLS.Clients = append(s.MTLS.Clients, c)
			}
		} else {
			ssl = append(ssl, c)
		}
	}
	for i := range s.Sites {
		site := &s.Sites[i]
		if site.ClientCAID != "" {
			copyCA(site.ClientCAID)
			exists := false
			for _, b := range s.MTLS.Bindings {
				if b.SiteID == site.ID {
					exists = true
				}
			}
			if !exists {
				s.MTLS.Bindings = append(s.MTLS.Bindings, MTLSBinding{SiteID: site.ID, CAID: site.ClientCAID, Enabled: true})
			}
			site.ClientCAID = ""
		}
	}
	if s.MTLS.NextID == 0 {
		s.MTLS.NextID = 1
	}
	if s.MTLS.CAs == nil {
		s.MTLS.CAs = []SSLCARecord{}
	}
	for _, c := range s.MTLS.Authorities {
		exists := false
		for _, r := range s.MTLS.CAs {
			if r.MaterialID == c.ID {
				exists = true
			}
		}
		if !exists {
			s.MTLS.CAs = append(s.MTLS.CAs, SSLCARecord{MaterialID: c.ID, WebsiteCA: model.WebsiteCA{BaseModel: model.BaseModel{ID: s.MTLS.NextID, CreatedAt: c.NotBefore, UpdatedAt: c.NotBefore}, Name: c.Name, KeyType: certificateKeyType(c)}})
			s.MTLS.NextID++
		}
	}
	s.Certificates = ssl
	if s.SSL.NextID == 0 {
		s.SSL.NextID = 1
	}
	if s.SSL.Records == nil {
		s.SSL.Records = []SSLRecord{}
	}
	if s.SSL.CAs == nil {
		s.SSL.CAs = []SSLCARecord{}
	}
	if s.SSL.Accounts == nil {
		s.SSL.Accounts = []SSLAccount{}
	}
	if s.SSL.DNSAccounts == nil {
		s.SSL.DNSAccounts = []SSLDNSAccount{}
	}
	for _, c := range s.Certificates {
		if c.Kind == "ca" {
			exists := false
			for _, r := range s.SSL.CAs {
				if r.MaterialID == c.ID {
					exists = true
				}
			}
			if !exists {
				r := SSLCARecord{MaterialID: c.ID, WebsiteCA: model.WebsiteCA{BaseModel: model.BaseModel{ID: s.SSL.NextID, CreatedAt: c.NotBefore, UpdatedAt: c.NotBefore}, Name: c.Name, KeyType: certificateKeyType(c)}}
				s.SSL.NextID++
				s.SSL.CAs = append(s.SSL.CAs, r)
			}
		}
		if c.Kind == "server" {
			exists := false
			for _, r := range s.SSL.Records {
				if r.MaterialID == c.ID {
					exists = true
				}
			}
			if !exists {
				domains := ""
				for i, d := range c.Domains {
					if i > 0 {
						if domains != "" {
							domains += ","
						}
						domains += d
					}
				}
				primary := c.Name
				if len(c.Domains) > 0 {
					primary = c.Domains[0]
				}
				r := SSLRecord{MaterialID: c.ID, WebsiteSSL: model.WebsiteSSL{BaseModel: model.BaseModel{ID: s.SSL.NextID, CreatedAt: c.NotBefore, UpdatedAt: c.NotBefore}, PrimaryDomain: primary, Domains: domains, Provider: "manual", Description: c.Name, Status: "ready", StartDate: c.NotBefore, ExpireDate: c.NotAfter, KeyType: certificateKeyType(c)}}
				s.SSL.NextID++
				s.SSL.Records = append(s.SSL.Records, r)
			}
		}
	}
}

func certificateKeyType(c Certificate) string {
	pair, e := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
	if e != nil {
		return ""
	}
	switch k := pair.PrivateKey.(type) {
	case *rsa.PrivateKey:
		return fmt.Sprintf("RSA%d", k.N.BitLen())
	case *ecdsa.PrivateKey:
		return fmt.Sprintf("EC%d", k.Curve.Params().BitSize)
	}
	return ""
}
