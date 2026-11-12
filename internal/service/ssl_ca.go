package service

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"slices"
	"strings"
	"time"
	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	panelssl "web-panel/internal/onepanel/ssl"
)

// CA creation/signing follows 1Panel's WebsiteCAService; only repository access is adapted.
func (m *Manager) CreateCA(v CACreate, mtls bool) error {
	if len(v.Name) > 128 || len(v.CommonName) > 128 || v.Name == "" || v.CommonName == "" || v.Country == "" || v.Organization == "" {
		return fmt.Errorf("CA 名称、通用名称、国家与组织不能为空")
	}
	key, public, keyPEM, e := panelssl.CreatePrivateKey(v.KeyType)
	if e != nil {
		return e
	}
	now := time.Now()
	subject := pkix.Name{CommonName: v.CommonName, Country: []string{v.Country}, Organization: []string{v.Organization}, OrganizationalUnit: []string{v.OrganizationUint}}
	if v.Province != "" {
		subject.Province = []string{v.Province}
	}
	if v.City != "" {
		subject.Locality = []string{v.City}
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return e
	}
	root := &x509.Certificate{SerialNumber: serial, Subject: subject, NotBefore: now, NotAfter: now.AddDate(10, 0, 0), BasicConstraintsValid: true, IsCA: true, MaxPathLen: 1, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, e := x509.CreateCertificate(rand.Reader, root, root, public, key)
	if e != nil {
		return e
	}
	return m.ImportCA(mtls, v.Name, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(keyPEM))
}
func (m *Manager) ImportCA(mtls bool, name, cert, key string) error {
	if strings.TrimSpace(name) == "" || len(name) > 128 || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("CA 名称必须为 1–128 字符")
	}
	parsed, e := panelssl.ParseRootCA([]byte(cert), []byte(key))
	if e != nil {
		return e
	}
	cert, key = parsed.CSR, parsed.PrivateKey
	c, e := materialFromPEM(name, cert, key, "ca", "")
	if e != nil {
		return e
	}
	return m.sslChange(func(s *domain.State) error {
		records, materials := caRepository(s, mtls)
		if slices.ContainsFunc(*records, func(r domain.SSLCARecord) bool { return r.Name == name }) {
			return fmt.Errorf("CA 名称已存在")
		}
		pair, _ := tls.X509KeyPair([]byte(cert), []byte(key))
		*records = append(*records, domain.SSLCARecord{WebsiteCA: model.WebsiteCA{BaseModel: caNext(s, mtls), Name: name, KeyType: keyType(pair.PrivateKey)}, MaterialID: c.ID})
		*materials = append(*materials, c)
		return nil
	})
}
func (m *Manager) CA(id uint, mtls bool) (map[string]any, error) {
	s, e := m.SSLList()
	if e != nil {
		return nil, e
	}
	records, materials := caRepository(&s, mtls)
	for _, r := range *records {
		if r.ID != id {
			continue
		}
		c := caMaterial(*materials, r.MaterialID)
		if c == nil {
			return nil, ErrNotFound
		}
		pair, _ := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		leaf, _ := x509.ParseCertificate(pair.Certificate[0])
		return map[string]any{"id": r.ID, "createdAt": r.CreatedAt, "name": r.Name, "keyType": r.KeyType, "csr": c.CertPEM, "privateKey": c.KeyPEM, "commonName": leaf.Subject.CommonName, "organization": strings.Join(leaf.Subject.Organization, ","), "organizationUint": strings.Join(leaf.Subject.OrganizationalUnit, ","), "country": strings.Join(leaf.Subject.Country, ","), "province": strings.Join(leaf.Subject.Province, ","), "city": strings.Join(leaf.Subject.Locality, ",")}, nil
	}
	return nil, ErrNotFound
}
func (m *Manager) DeleteCA(id uint, mtls bool) error {
	return m.sslChange(func(s *domain.State) error {
		if mtls {
			return deleteMTLSCA(s, id)
		}
		for _, r := range s.SSL.Records {
			if r.CaID == id {
				return fmt.Errorf("CA 仍有已签发的 SSL 证书")
			}
		}
		i := slices.IndexFunc(s.SSL.CAs, func(r domain.SSLCARecord) bool { return r.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		if s.SSL.CAs[i].Name == "1Panel" {
			return fmt.Errorf("不能删除默认 CA")
		}
		material := s.SSL.CAs[i].MaterialID
		s.SSL.CAs = slices.Delete(s.SSL.CAs, i, i+1)
		s.Certificates = slices.DeleteFunc(s.Certificates, func(c domain.Certificate) bool { return c.ID == material })
		return nil
	})
}
func (m *Manager) SSLCreateCA(v CACreate) error { return m.CreateCA(v, false) }
func (m *Manager) SSLImportCA(name, cert, key string) error {
	return m.ImportCA(false, name, cert, key)
}
func (m *Manager) SSLCA(id uint) (map[string]any, error) { return m.CA(id, false) }
func (m *Manager) SSLDeleteCA(id uint) error             { return m.DeleteCA(id, false) }
func caRepository(s *domain.State, mtls bool) (*[]domain.SSLCARecord, *[]domain.Certificate) {
	if mtls {
		return &s.MTLS.CAs, &s.MTLS.Authorities
	}
	return &s.SSL.CAs, &s.Certificates
}
func caMaterial(materials []domain.Certificate, id string) *domain.Certificate {
	for i := range materials {
		if materials[i].ID == id {
			return &materials[i]
		}
	}
	return nil
}
func caNext(s *domain.State, mtls bool) model.BaseModel {
	if !mtls {
		return sslNext(s)
	}
	now := time.Now().UTC()
	id := s.MTLS.NextID
	s.MTLS.NextID++
	return model.BaseModel{ID: id, CreatedAt: now, UpdatedAt: now}
}
func deleteMTLSCA(s *domain.State, id uint) error {
	i := slices.IndexFunc(s.MTLS.CAs, func(r domain.SSLCARecord) bool { return r.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	material := s.MTLS.CAs[i].MaterialID
	if slices.ContainsFunc(s.MTLS.Clients, func(c domain.Certificate) bool { return c.CAID == material && c.RevokedAt == nil }) {
		return fmt.Errorf("请先吊销此 CA 的有效客户端")
	}
	if slices.ContainsFunc(s.MTLS.Bindings, func(b domain.MTLSBinding) bool { return b.CAID == material }) {
		return fmt.Errorf("请先解除此 CA 的网站绑定")
	}
	s.MTLS.CAs = slices.Delete(s.MTLS.CAs, i, i+1)
	s.MTLS.Authorities = slices.DeleteFunc(s.MTLS.Authorities, func(c domain.Certificate) bool { return c.ID == material })
	s.MTLS.Clients = slices.DeleteFunc(s.MTLS.Clients, func(c domain.Certificate) bool { return c.CAID == material })
	return nil
}
func (m *Manager) SSLSign(v SSLCommand) (uint, error) {
	var id uint
	err := m.sslChange(func(s *domain.State) error {
		var ca *domain.Certificate
		caID := v.CaID
		if caID == 0 {
			caID = v.ID
		}
		if v.Renew {
			if old := sslRecord(s, v.SSLID); old != nil {
				caID = old.CaID
				v.PrimaryDomain = old.PrimaryDomain
				v.Domains = old.Domains
				v.Time = old.Validity
				v.Unit = old.ValidityUnit
				v.WebsiteSSL = old.WebsiteSSL
			}
		}
		for _, r := range s.SSL.CAs {
			if r.ID == caID {
				ca = sslMaterial(s, r.MaterialID)
			}
		}
		if ca == nil {
			return ErrNotFound
		}
		domains := sslDomains(v.PrimaryDomain, v.Domains+"\n"+v.OtherDomains)
		if e := domain.ValidateDomains(domains); e != nil {
			return e
		}
		if v.Time < 1 || v.Time > 3650 || v.Unit != "year" && v.Unit != "day" {
			return fmt.Errorf("有效期无效")
		}
		if e := validateSSLDeployment(v.WebsiteSSL); e != nil {
			return e
		}
		pair, e := tls.X509KeyPair([]byte(ca.CertPEM), []byte(ca.KeyPEM))
		if e != nil {
			return e
		}
		root, _ := x509.ParseCertificate(pair.Certificate[0])
		private, public, keyPEM, e := panelssl.CreatePrivateKey(v.KeyType)
		_ = private
		if e != nil {
			return e
		}
		now := time.Now()
		end := now.AddDate(0, 0, v.Time)
		if v.Unit == "year" {
			end = now.AddDate(v.Time, 0, 0)
		}
		if end.After(root.NotAfter) {
			end = root.NotAfter
		}
		if !now.Before(end) {
			return fmt.Errorf("CA 不在有效期内")
		}
		serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if e != nil {
			return e
		}
		subject := root.Subject
		subject.CommonName = domains[0]
		leaf := &x509.Certificate{SerialNumber: serial, Subject: subject, NotBefore: now.Add(-time.Minute), NotAfter: end, BasicConstraintsValid: true, KeyUsage: panelssl.LeafKeyUsage(v.KeyType), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		for _, d := range domains {
			if ip := net.ParseIP(d); ip != nil {
				leaf.IPAddresses = append(leaf.IPAddresses, ip)
			} else {
				leaf.DNSNames = append(leaf.DNSNames, d)
			}
		}
		der, e := x509.CreateCertificate(rand.Reader, leaf, root, public, pair.PrivateKey)
		if e != nil {
			return e
		}
		r := sslRecord(s, v.SSLID)
		if v.Renew && r == nil {
			return ErrNotFound
		}
		if r == nil {
			record := v.WebsiteSSL
			record.BaseModel = sslNext(s)
			record.Provider = "selfSigned"
			record.CaID = caID
			s.SSL.Records = append(s.SSL.Records, domain.SSLRecord{WebsiteSSL: record, Validity: v.Time, ValidityUnit: v.Unit})
			r = &s.SSL.Records[len(s.SSL.Records)-1]
		}
		r.PrimaryDomain = domains[0]
		r.Domains = strings.Join(domains[1:], ",")
		if e = saveSSLMaterial(s, r, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(keyPEM)); e != nil {
			return e
		}
		sslMaterial(s, r.MaterialID).CAID = ca.ID
		id = r.ID
		return nil
	})
	if err == nil {
		m.sslLog(id, "SSL 自签证书签发成功")
		m.sslDeploy(id)
	}
	return id, err
}
