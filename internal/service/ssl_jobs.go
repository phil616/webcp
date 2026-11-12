package service

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-acme/lego/v5/certificate"
	legolog "github.com/go-acme/lego/v5/log"
	"github.com/go-acme/lego/v5/providers/http/webroot"
	"web-panel/internal/domain"
	panelssl "web-panel/internal/onepanel/ssl"
	"web-panel/internal/store"
)

var sslLegoLogMu sync.Mutex

func (m *Manager) sslLogPath(id uint) string {
	return filepath.Join(m.store.Root, "ssl", "logs", fmt.Sprintf("%d.log", id))
}
func (m *Manager) sslLog(id uint, message string) {
	os.MkdirAll(filepath.Dir(m.sslLogPath(id)), 0700)
	f, e := os.OpenFile(m.sslLogPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		defer f.Close()
		fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), message)
	}
}
func (m *Manager) SSLLog(id uint) ([]string, error) {
	data, e := os.ReadFile(m.sslLogPath(id))
	if os.IsNotExist(e) {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n"), e
}
func (m *Manager) SSLResolve(ctx context.Context, id uint) ([]map[string]string, error) {
	s, e := m.SSLList()
	if e != nil {
		return nil, e
	}
	r := sslRecord(&s, id)
	if r == nil {
		return nil, ErrNotFound
	}
	account, e := sslAccount(&s, r.AcmeAccountID)
	if e != nil {
		return nil, e
	}
	proxy, e := sslProxy(account.UseProxy)
	if e != nil {
		return nil, e
	}
	client, e := panelssl.NewCustomAcmeClient(&account, proxy, log.New(io.Discard, "", 0))
	if e != nil {
		return nil, e
	}
	model := sslModel(&s, *r, true)
	records, e := client.GetDNSResolve(ctx, &model)
	if e != nil {
		return nil, e
	}
	result := []map[string]string{}
	for domain, r := range records {
		result = append(result, map[string]string{"domain": domain, "resolve": r.Key, "value": r.Value, "err": r.Err})
	}
	slices.SortFunc(result, func(a, b map[string]string) int { return strings.Compare(a["domain"], b["domain"]) })
	return result, nil
}
func (m *Manager) SSLObtain(id uint) error {
	s, e := m.SSLList()
	if e != nil {
		return e
	}
	r := sslRecord(&s, id)
	if r == nil {
		return ErrNotFound
	}
	if r.Provider == "manual" {
		return fmt.Errorf("手动导入证书不支持自动申请")
	}
	if r.Status == "applying" {
		return fmt.Errorf("证书正在申请中")
	}
	if e = m.sslChange(func(s *domain.State) error {
		r := sslRecord(s, id)
		if r == nil {
			return ErrNotFound
		}
		if r.Status == "applying" {
			return fmt.Errorf("证书正在申请中")
		}
		r.Status = "applying"
		r.Message = ""
		return nil
	}); e != nil {
		return e
	}
	go m.sslObtainJob(id)
	return nil
}
func (m *Manager) sslObtainJob(id uint) {
	ctx, cancel := context.WithTimeout(m.sslContext(), time.Hour)
	defer cancel()
	m.sslLog(id, "开始申请 SSL 证书")
	s, e := m.SSLList()
	if e != nil {
		return
	}
	record := sslRecord(&s, id)
	if record == nil {
		return
	}
	r := *record
	if r.Provider == "selfSigned" {
		_, e = m.SSLSign(SSLCommand{SSLID: id, Renew: true})
		if e != nil {
			m.sslJobError(id, e)
		}
		return
	}
	account, e := sslAccount(&s, r.AcmeAccountID)
	if e != nil {
		m.sslJobError(id, e)
		return
	}
	proxy, e := sslProxy(account.UseProxy)
	if e != nil {
		m.sslJobError(id, e)
		return
	}
	var resource certificate.Resource
	os.MkdirAll(filepath.Dir(m.sslLogPath(id)), 0700)
	file, e := os.OpenFile(m.sslLogPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		m.sslJobError(id, e)
		return
	}
	defer file.Close()
	logger := log.New(file, "", log.LstdFlags)
	if r.Provider == "dnsManual" {
		var client *panelssl.ManualClient
		client, e = panelssl.NewCustomAcmeClient(&account, proxy, logger)
		if e == nil {
			v := sslModel(&s, r, true)
			resource, e = client.RequestCertificate(ctx, &v)
		}
	} else {
		sslLegoLogMu.Lock()
		old := legolog.Default()
		legolog.SetDefault(slog.New(slog.NewTextHandler(file, nil)))
		func() {
			defer func() { legolog.SetDefault(old); sslLegoLogMu.Unlock() }()
			client, err := panelssl.NewAcmeClientWithContext(ctx, &account, proxy)
			if err != nil {
				e = err
				return
			}
			if r.Provider == "dnsAccount" {
				for _, dns := range s.SSL.DNSAccounts {
					if dns.ID == r.DnsAccountID {
						e = client.UseDns(panelssl.DnsType(dns.Type), dns.Secret, r.WebsiteSSL)
						break
					}
				}
			} else {
				webrootPath := filepath.Join(m.store.Root, "runtime", "acme")
				e = os.MkdirAll(webrootPath, 0700)
				if e == nil {
					var provider *webroot.HTTPProvider
					provider, e = webroot.NewHTTPProvider(webrootPath)
					if e == nil {
						e = client.Client.Challenge.SetHTTP01Provider(&syncedHTTPChallenge{provider: provider, sync: func(ctx context.Context, name string, remove bool) error {
							return m.syncRuntimeFile(ctx, "acme", "", name, remove)
						}})
					}
				}
				if e == nil {
					status := m.runtime.Status(ctx)
					if !status.Running {
						e = fmt.Errorf("请先启动 OpenResty，再使用 HTTP 验证")
					} else {
						e = m.Apply(ctx, s.Revision)
					}
				}
			}
			if e != nil {
				return
			}
			var key crypto.Signer
			key, e = panelssl.GetPrivateKeyByType(r.KeyType, sslModel(&s, r, true).PrivateKey)
			if e != nil {
				return
			}
			domains := sslDomains(r.PrimaryDomain, r.Domains)
			if r.IsIp {
				resource, e = client.ObtainIPSSL(ctx, r.PrimaryDomain, key)
			} else {
				resource, e = client.ObtainSSL(ctx, domains, key)
			}
		}()
	}
	if e != nil {
		m.sslJobError(id, e)
		return
	}
	e = m.sslChange(func(s *domain.State) error {
		current := sslRecord(s, id)
		if current == nil {
			return ErrNotFound
		}
		if e := saveSSLMaterial(s, current, string(resource.Certificate), string(resource.PrivateKey)); e != nil {
			return e
		}
		current.CertURL = resource.CertURL
		return nil
	})
	if e != nil {
		m.sslJobError(id, e)
		return
	}
	m.sslLog(id, "SSL 证书申请成功")
	m.sslDeploy(id)
}
func (m *Manager) sslJobError(id uint, e error) {
	message := e.Error()
	s, _ := m.SSLList()
	for _, a := range s.SSL.Accounts {
		for _, secret := range []string{a.Secret, a.EabHmacKey} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[REDACTED]")
			}
		}
	}
	for _, a := range s.SSL.DNSAccounts {
		var values map[string]string
		json.Unmarshal([]byte(a.Secret), &values)
		for _, secret := range values {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[REDACTED]")
			}
		}
	}
	m.sslLog(id, "申请失败: "+message)
	_ = m.sslChange(func(s *domain.State) error {
		r := sslRecord(s, id)
		if r == nil {
			return ErrNotFound
		}
		r.Status = "error"
		r.Message = message
		return nil
	})
}
func (m *Manager) sslDeploy(id uint) {
	s, e := m.SSLList()
	if e != nil {
		return
	}
	record := sslRecord(&s, id)
	if record == nil {
		return
	}
	r := *record
	material := sslMaterial(&s, r.MaterialID)
	if material == nil {
		return
	}
	if r.PushDir {
		for _, dir := range strings.Split(r.Dir, "\n") {
			if e = store.AtomicWrite(filepath.Join(dir, "fullchain.pem"), []byte(material.CertPEM), 0600); e == nil {
				e = store.AtomicWrite(filepath.Join(dir, "privkey.pem"), []byte(material.KeyPEM), 0600)
			}
			if e != nil {
				m.sslLog(id, "推送失败: "+e.Error())
				return
			}
		}
	}
	if r.ExecShell && r.Shell != "" {
		ctx, cancel := context.WithTimeout(m.sslContext(), 30*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, "/bin/sh", "-s")
		command.Stdin = strings.NewReader(r.Shell)
		command.Dir = m.store.Root
		if r.PushDir {
			command.Dir = strings.Split(r.Dir, "\n")[0]
		}
		out, e := command.CombinedOutput()
		m.sslLog(id, string(out))
		if e != nil {
			m.sslLog(id, "执行脚本失败: "+e.Error())
		}
	}
	if e = m.sslReloadBound(id); e != nil {
		m.sslLog(id, "部署失败: "+e.Error())
	}
}
func (m *Manager) sslReloadBound(id uint) error {
	s, e := m.SSLList()
	if e != nil {
		return e
	}
	r := sslRecord(&s, id)
	if r == nil {
		return ErrNotFound
	}
	if !slices.ContainsFunc(s.Sites, func(site domain.Site) bool { return site.TLSID == r.MaterialID }) {
		return nil
	}
	ctx, cancel := context.WithTimeout(m.sslContext(), 2*time.Minute)
	defer cancel()
	if m.runtime.Status(ctx).Running {
		return m.Apply(ctx, s.Revision)
	}
	return nil
}

func (m *Manager) StartSSLWorker(ctx context.Context) {
	m.mu.Lock()
	m.jobContext = ctx
	m.mu.Unlock()
	s, e := m.SSLList()
	if e == nil && slices.ContainsFunc(s.SSL.Records, func(r domain.SSLRecord) bool { return r.Status == "applying" }) {
		_ = m.sslChange(func(s *domain.State) error {
			for i := range s.SSL.Records {
				if s.SSL.Records[i].Status == "applying" {
					s.SSL.Records[i].Status = "systemRestart"
					s.SSL.Records[i].Message = "管理服务重启，未完成的申请需要重新发起"
				}
			}
			return nil
		})
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			m.sslRenewDue()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (m *Manager) sslRenewDue() {
	s, e := m.SSLList()
	if e != nil {
		return
	}
	for _, r := range s.SSL.Records {
		if !r.AutoRenew || r.Status == "applying" || r.Provider == "manual" || r.Provider == "dnsManual" {
			continue
		}
		threshold := 30 * 24 * time.Hour
		lifetime := r.ExpireDate.Sub(r.StartDate)
		if lifetime > 0 && lifetime/3 < threshold {
			threshold = lifetime / 3
		}
		if time.Until(r.ExpireDate) <= threshold {
			_ = m.SSLObtain(r.ID)
		}
	}
}

func (m *Manager) sslContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobContext == nil {
		return context.Background()
	}
	return m.jobContext
}
