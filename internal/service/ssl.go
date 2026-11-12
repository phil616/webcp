package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	panelssl "web-panel/internal/onepanel/ssl"
)

type SSLCommand struct {
	model.WebsiteSSL
	OtherDomains    string            `json:"otherDomains"`
	WebsiteID       uint              `json:"websiteId"`
	Time            int               `json:"time"`
	Unit            string            `json:"unit"`
	Apply           bool              `json:"apply"`
	SSLID           uint              `json:"sslID"`
	IDUpper         uint              `json:"ID"`
	Certificate     string            `json:"certificate"`
	CertificatePath string            `json:"certificatePath"`
	Renew           bool              `json:"renew"`
	TXTRecords      map[string]string `json:"TXTRecords"`
	PushNodes       []string          `json:"pushNodes"`
}
type CACreate struct {
	Name             string `json:"name"`
	CommonName       string `json:"commonName"`
	Country          string `json:"country"`
	Organization     string `json:"organization"`
	OrganizationUint string `json:"organizationUint"`
	Province         string `json:"province"`
	City             string `json:"city"`
	KeyType          string `json:"keyType"`
}
type DNSCommand struct {
	ID            uint              `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Authorization map[string]string `json:"authorization"`
}

func (m *Manager) sslChange(fn func(*domain.State) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.store.Load()
	if e != nil {
		return e
	}
	if e = fn(&s); e != nil {
		return e
	}
	domain.NormalizeState(&s)
	if e = domain.Validate(s); e != nil {
		return e
	}
	s.Revision++
	return m.store.Commit(s.Revision-1, s)
}
func sslNext(s *domain.State) model.BaseModel {
	id := s.SSL.NextID
	s.SSL.NextID++
	now := time.Now().UTC()
	return model.BaseModel{ID: id, CreatedAt: now, UpdatedAt: now}
}
func sslMaterial(s *domain.State, id string) *domain.Certificate {
	for i := range s.Certificates {
		if s.Certificates[i].ID == id {
			return &s.Certificates[i]
		}
	}
	return nil
}
func sslRecord(s *domain.State, id uint) *domain.SSLRecord {
	for i := range s.SSL.Records {
		if s.SSL.Records[i].ID == id {
			return &s.SSL.Records[i]
		}
	}
	return nil
}
func sslModel(s *domain.State, r domain.SSLRecord, private bool) model.WebsiteSSL {
	v := r.WebsiteSSL
	if c := sslMaterial(s, r.MaterialID); c != nil {
		v.Pem = c.CertPEM
		if private {
			v.PrivateKey = c.KeyPEM
		}
	}
	v.Websites = []model.Website{}
	for _, site := range s.Sites {
		if site.TLSID == r.MaterialID {
			v.Websites = append(v.Websites, model.Website{ID: site.PanelID, PrimaryDomain: site.Domains[0], Alias: site.Name})
		}
	}
	for _, a := range s.SSL.Accounts {
		if a.ID == v.AcmeAccountID {
			v.AcmeAccount = a.WebsiteAcmeAccount
			v.AcmeAccount.EabHmacKey = ""
		}
	}
	for _, d := range s.SSL.DNSAccounts {
		if d.ID == v.DnsAccountID {
			v.DnsAccount = d.WebsiteDnsAccount
		}
	}
	return v
}
func (m *Manager) SSLList() (domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Load()
}
func (m *Manager) SSLGet(id uint) (model.WebsiteSSL, error) {
	s, e := m.SSLList()
	if e != nil {
		return model.WebsiteSSL{}, e
	}
	r := sslRecord(&s, id)
	if r == nil {
		return model.WebsiteSSL{}, ErrNotFound
	}
	return sslModel(&s, *r, true), nil
}
func keyType(key any) string {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		if k.Curve.Params().BitSize == 384 {
			return "EC384"
		}
		return "EC256"
	case *rsa.PrivateKey:
		return fmt.Sprintf("RSA%d", k.N.BitLen())
	}
	return ""
}
func materialFromPEM(name, cert, key, kind, id string) (domain.Certificate, error) {
	pair, e := tls.X509KeyPair([]byte(cert), []byte(key))
	if e != nil {
		return domain.Certificate{}, e
	}
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		return domain.Certificate{}, e
	}
	if (kind == "ca") != leaf.IsCA {
		return domain.Certificate{}, fmt.Errorf("证书用途不匹配")
	}
	if kind == "server" && len(leaf.ExtKeyUsage) > 0 && !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth) && !slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageAny) {
		return domain.Certificate{}, fmt.Errorf("证书用途不包含 serverAuth")
	}
	if id == "" {
		id = domain.NewID()
	}
	domains := slices.Clone(leaf.DNSNames)
	for _, ip := range leaf.IPAddresses {
		domains = append(domains, ip.String())
	}
	return domain.Certificate{ID: id, Name: name, Kind: kind, Serial: leaf.SerialNumber.String(), Domains: domains, CertPEM: cert, KeyPEM: key, NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter}, nil
}
func saveSSLMaterial(s *domain.State, r *domain.SSLRecord, cert, key string) error {
	c, e := materialFromPEM(r.PrimaryDomain, cert, key, "server", r.MaterialID)
	if e != nil {
		return e
	}
	if len(c.Domains) == 0 {
		return fmt.Errorf("证书没有域名或 IP SAN")
	}
	r.MaterialID = c.ID
	r.StartDate = c.NotBefore
	r.ExpireDate = c.NotAfter
	r.Status = "ready"
	r.Message = ""
	r.UpdatedAt = time.Now().UTC()
	pair, _ := tls.X509KeyPair([]byte(cert), []byte(key))
	r.KeyType = keyType(pair.PrivateKey)
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])
	r.Type = leaf.Issuer.CommonName
	r.Organization = strings.Join(leaf.Issuer.Organization, ",")
	if old := sslMaterial(s, c.ID); old != nil {
		c.CAID = old.CAID
		*old = c
	} else {
		s.Certificates = append(s.Certificates, c)
	}
	r.Pem = ""
	r.PrivateKey = ""
	return nil
}
func (m *Manager) SSLUpload(v SSLCommand) error {
	cert, key := v.Certificate, v.PrivateKey
	if v.Type == "local" {
		var e error
		cert, e = ReadPEMFile(v.CertificatePath)
		if e != nil {
			return e
		}
		key, e = ReadPEMFile(v.PrivateKeyPath)
		if e != nil {
			return e
		}
	} else if v.Type != "paste" && v.Type != "upload" {
		return fmt.Errorf("导入方式无效")
	}
	var savedID uint
	err := m.sslChange(func(s *domain.State) error {
		id := v.SSLID
		if id == 0 {
			id = v.ID
		}
		r := sslRecord(s, id)
		if id > 0 && r == nil {
			return ErrNotFound
		}
		if r == nil {
			model := model.WebsiteSSL{BaseModel: sslNext(s), Provider: "manual", Description: v.Description}
			s.SSL.Records = append(s.SSL.Records, domain.SSLRecord{WebsiteSSL: model})
			r = &s.SSL.Records[len(s.SSL.Records)-1]
		}
		c, e := materialFromPEM("import", cert, key, "server", r.MaterialID)
		if e != nil {
			return e
		}
		if len(c.Domains) == 0 {
			return fmt.Errorf("证书缺少 SAN")
		}
		r.PrimaryDomain = c.Domains[0]
		r.Domains = strings.Join(c.Domains[1:], ",")
		r.Description = v.Description
		r.CertPath = v.CertificatePath
		r.PrivateKeyPath = v.PrivateKeyPath
		savedID = r.ID
		if r.Provider == "manual" {
			if old := sslMaterial(s, r.MaterialID); old != nil {
				old.CAID = ""
			}
		}
		return saveSSLMaterial(s, r, cert, key)
	})
	if err != nil {
		return err
	}
	if err = m.sslReloadBound(savedID); err != nil {
		return fmt.Errorf("证书已保存，但部署失败: %w", err)
	}
	return nil
}
func ReadPEMFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", fmt.Errorf("证书文件必须是 1 MiB 以内的普通文件")
	}
	b, e := io.ReadAll(io.LimitReader(f, 1<<20))
	return string(b), e
}
func (m *Manager) SSLSave(v SSLCommand, create bool) (uint, error) {
	var id uint
	e := m.sslChange(func(s *domain.State) error {
		r := sslRecord(s, v.ID)
		if create {
			if !slices.Contains([]string{"dnsAccount", "dnsManual", "http"}, v.Provider) {
				return fmt.Errorf("申请方式无效")
			}
			s.SSL.Records = append(s.SSL.Records, domain.SSLRecord{WebsiteSSL: model.WebsiteSSL{BaseModel: sslNext(s)}})
			r = &s.SSL.Records[len(s.SSL.Records)-1]
		} else if r == nil {
			return ErrNotFound
		}
		if r.Status == "applying" {
			return fmt.Errorf("证书正在申请中")
		}
		if !slices.Contains([]string{"dnsAccount", "dnsManual", "http", "selfSigned", "manual"}, v.Provider) {
			return fmt.Errorf("申请方式无效")
		}
		if v.Provider != "manual" && !slices.Contains([]string{"EC256", "EC384", "RSA2048", "RSA3072", "RSA4096", ""}, v.KeyType) {
			return fmt.Errorf("密钥算法无效")
		}
		if v.PrimaryDomain == "" {
			return fmt.Errorf("需要主域名")
		}
		domains := sslDomains(v.PrimaryDomain, v.OtherDomains)
		if e := domain.ValidateDomains(domains); e != nil {
			return e
		}
		if e := validateSSLDeployment(v.WebsiteSSL); e != nil {
			return e
		}
		r.PrimaryDomain = domains[0]
		r.Domains = strings.Join(domains[1:], ",")
		r.Provider = v.Provider
		r.AcmeAccountID = v.AcmeAccountID
		r.DnsAccountID = v.DnsAccountID
		r.AutoRenew = v.AutoRenew && v.Provider != "dnsManual" && v.Provider != "manual"
		r.Description = v.Description
		r.SkipDNS = v.SkipDNS
		r.DisableCNAME = v.DisableCNAME
		r.Nameserver1 = v.Nameserver1
		r.Nameserver2 = v.Nameserver2
		r.KeyType = v.KeyType
		r.PushDir = v.PushDir
		r.Dir = v.Dir
		r.ExecShell = v.ExecShell
		r.Shell = v.Shell
		r.IsIp = v.IsIp
		r.UpdatedAt = time.Now().UTC()
		if r.KeyType == "" {
			r.KeyType = "EC256"
		}
		if r.Status == "" {
			r.Status = "init"
		}
		if v.Provider != "selfSigned" && v.Provider != "manual" {
			if !slices.ContainsFunc(s.SSL.Accounts, func(a domain.SSLAccount) bool { return a.ID == v.AcmeAccountID }) {
				return fmt.Errorf("请选择有效 ACME 账户")
			}
			if v.Provider == "dnsAccount" && !slices.ContainsFunc(s.SSL.DNSAccounts, func(a domain.SSLDNSAccount) bool { return a.ID == v.DnsAccountID }) {
				return fmt.Errorf("请选择有效 DNS 账户")
			}
		}
		id = r.ID
		return nil
	})
	return id, e
}
func sslDomains(primary, others string) []string {
	out := []string{}
	for _, d := range strings.FieldsFunc(primary+"\n"+others, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == ' ' }) {
		d = strings.ToLower(strings.TrimSpace(d))
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}
func validateSSLDeployment(v model.WebsiteSSL) error {
	if len(v.Description) > 128 {
		return fmt.Errorf("备注最多 128 字符")
	}
	if v.PushDir {
		for _, dir := range strings.Split(v.Dir, "\n") {
			info, e := os.Stat(dir)
			if e != nil || !filepath.IsAbs(dir) || !info.IsDir() {
				return fmt.Errorf("推送目录不存在或不是绝对目录: %s", dir)
			}
		}
	}
	if len(v.Shell) > 256<<10 {
		return fmt.Errorf("shell 脚本过长")
	}
	return nil
}
func (m *Manager) SSLDelete(ctx context.Context, ids []uint) error {
	s, e := m.SSLList()
	if e != nil {
		return e
	}
	for _, id := range ids {
		r := sslRecord(&s, id)
		if r == nil {
			return ErrNotFound
		}
		if r.Status == "applying" {
			return fmt.Errorf("证书申请中，不能删除")
		}
		if slices.ContainsFunc(s.Sites, func(site domain.Site) bool { return site.TLSID == r.MaterialID && r.MaterialID != "" }) {
			return fmt.Errorf("证书仍被网站引用")
		}
		if slices.Contains([]string{"dnsAccount", "dnsManual", "http"}, r.Provider) && r.MaterialID != "" {
			a, e := sslAccount(&s, r.AcmeAccountID)
			if e != nil {
				return e
			}
			proxy, e := sslProxy(a.UseProxy)
			if e != nil {
				return e
			}
			client, e := panelssl.NewAcmeClientWithContext(ctx, &a, proxy)
			if e != nil {
				return e
			}
			c := sslMaterial(&s, r.MaterialID)
			if e = client.RevokeSSL([]byte(c.CertPEM)); e != nil {
				return e
			}
		}
	}
	return m.sslChange(func(s *domain.State) error {
		for _, id := range ids {
			r := sslRecord(s, id)
			if r == nil {
				return ErrNotFound
			}
			material := r.MaterialID
			s.SSL.Records = slices.DeleteFunc(s.SSL.Records, func(r domain.SSLRecord) bool { return r.ID == id })
			s.Certificates = slices.DeleteFunc(s.Certificates, func(c domain.Certificate) bool { return c.ID == material })
		}
		return nil
	})
}
