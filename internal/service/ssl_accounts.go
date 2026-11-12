package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"slices"
	"time"
	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	panelssl "web-panel/internal/onepanel/ssl"
)

func (m *Manager) SSLSaveDNS(v DNSCommand) error {
	return m.sslChange(func(s *domain.State) error {
		if v.Name == "" || len(v.Name) > 128 || len(v.Authorization) == 0 {
			return fmt.Errorf("DNS 名称与授权信息不能为空")
		}
		blob, e := json.Marshal(v.Authorization)
		if e != nil {
			return e
		}
		if e = panelssl.ValidateDNSAccount(v.Type, string(blob)); e != nil {
			return e
		}
		if slices.ContainsFunc(s.SSL.DNSAccounts, func(a domain.SSLDNSAccount) bool { return a.ID != v.ID && a.Name == v.Name }) {
			return fmt.Errorf("DNS 账户名称已存在")
		}
		if v.ID == 0 {
			s.SSL.DNSAccounts = append(s.SSL.DNSAccounts, domain.SSLDNSAccount{WebsiteDnsAccount: model.WebsiteDnsAccount{BaseModel: sslNext(s), Name: v.Name, Type: v.Type}, Secret: string(blob)})
			return nil
		}
		for i := range s.SSL.DNSAccounts {
			if s.SSL.DNSAccounts[i].ID == v.ID {
				s.SSL.DNSAccounts[i].UpdatedAt = time.Now().UTC()
				s.SSL.DNSAccounts[i].Name = v.Name
				s.SSL.DNSAccounts[i].Type = v.Type
				s.SSL.DNSAccounts[i].Secret = string(blob)
				return nil
			}
		}
		return ErrNotFound
	})
}
func (m *Manager) SSLRegister(ctx context.Context, a model.WebsiteAcmeAccount) error {
	if _, e := mail.ParseAddress(a.Email); e != nil {
		return e
	}
	if a.Type == "custom" && a.CaDirURL == "" {
		return fmt.Errorf("需要自定义 ACME 目录 URL")
	}
	proxy, e := sslProxy(a.UseProxy)
	if e != nil {
		return e
	}
	client, e := panelssl.NewRegisterClientWithContext(ctx, &a, proxy)
	if e != nil {
		return e
	}
	a.URL = client.User.GetRegistration().Location
	return m.sslChange(func(s *domain.State) error {
		a.BaseModel = sslNext(s)
		s.SSL.Accounts = append(s.SSL.Accounts, domain.SSLAccount{WebsiteAcmeAccount: a, Secret: a.PrivateKey})
		return nil
	})
}
func (m *Manager) SSLDeleteAccount(id uint, dns bool) error {
	return m.sslChange(func(s *domain.State) error {
		for _, r := range s.SSL.Records {
			if (dns && r.DnsAccountID == id) || (!dns && r.AcmeAccountID == id) {
				return fmt.Errorf("账户仍被 SSL 证书引用")
			}
		}
		if dns {
			s.SSL.DNSAccounts = slices.DeleteFunc(s.SSL.DNSAccounts, func(a domain.SSLDNSAccount) bool { return a.ID == id })
		} else {
			s.SSL.Accounts = slices.DeleteFunc(s.SSL.Accounts, func(a domain.SSLAccount) bool { return a.ID == id })
		}
		return nil
	})
}
func sslAccount(s *domain.State, id uint) (model.WebsiteAcmeAccount, error) {
	for _, a := range s.SSL.Accounts {
		if a.ID == id {
			v := a.WebsiteAcmeAccount
			v.PrivateKey = a.Secret
			return v, nil
		}
	}
	return model.WebsiteAcmeAccount{}, ErrNotFound
}

func (m *Manager) SSLUpdateAccount(v model.WebsiteAcmeAccount) error {
	if _, e := sslProxy(v.UseProxy); e != nil {
		return e
	}
	return m.sslChange(func(s *domain.State) error {
		for i := range s.SSL.Accounts {
			if s.SSL.Accounts[i].ID == v.ID {
				s.SSL.Accounts[i].UseProxy = v.UseProxy
				return nil
			}
		}
		return ErrNotFound
	})
}
