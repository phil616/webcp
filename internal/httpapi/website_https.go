package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	"web-panel/internal/service"
)

func panelHTTPS(w http.ResponseWriter, r *http.Request, m *service.Manager, path string) error {
	s, e := m.SSLList()
	if e != nil {
		return e
	}
	id := uint(parseUint(strings.Split(path, "/")[1]))
	var site *domain.Site
	for i := range s.Sites {
		if s.Sites[i].PanelID == id {
			site = &s.Sites[i]
		}
	}
	if site == nil {
		return service.ErrNotFound
	}
	if r.Method == "GET" {
		cfg := domain.SiteDefaults(*site).HTTPS
		var cert model.WebsiteSSL
		for _, ssl := range s.SSL.Records {
			if ssl.MaterialID == site.TLSID {
				cert, _ = m.SSLGet(ssl.ID)
			}
		}
		writeJSON(w, 200, map[string]any{"enable": site.TLSID != "", "SSL": cert, "httpConfig": cfg.HTTPConfig, "hsts": cfg.HSTS, "hstsIncludeSubDomains": cfg.HSTSIncludeSubDomains, "SSLProtocol": cfg.Protocols, "algorithm": cfg.Ciphers, "httpsPort": strconv.Itoa(m.HTTPSPort()), "http3": cfg.HTTP3})
		return nil
	}
	var v struct {
		Enable                bool     `json:"enable"`
		WebsiteSSLID          uint     `json:"websiteSSLId"`
		HTTPConfig            string   `json:"httpConfig"`
		HSTS                  bool     `json:"hsts"`
		HSTSIncludeSubDomains bool     `json:"hstsIncludeSubDomains"`
		Protocols             []string `json:"SSLProtocol"`
		Algorithm             string   `json:"algorithm"`
		HTTP3                 bool     `json:"http3"`
		Type                  string   `json:"type"`
		Certificate           string   `json:"certificate"`
		PrivateKey            string   `json:"privateKey"`
		ImportType            string   `json:"importType"`
		CertificatePath       string   `json:"certificatePath"`
		PrivateKeyPath        string   `json:"privateKeyPath"`
	}
	if e = panelDecode(r, &v); e != nil {
		return e
	}
	if v.Enable && v.Type == "manual" {
		e = m.SSLUpload(service.SSLCommand{WebsiteSSL: model.WebsiteSSL{PrivateKey: v.PrivateKey, PrivateKeyPath: v.PrivateKeyPath, Type: v.ImportType}, Certificate: v.Certificate, CertificatePath: v.CertificatePath})
		if e != nil {
			return e
		}
		s, _ = m.SSLList()
		v.WebsiteSSLID = s.SSL.Records[len(s.SSL.Records)-1].ID
	}
	material := ""
	if v.Enable {
		record := findSSL(s.SSL.Records, v.WebsiteSSLID)
		if record == nil || record.MaterialID == "" {
			return fmt.Errorf("请选择已经签发的 SSL 证书")
		}
		material = record.MaterialID
	}
	update := service.HTTPSUpdate{TLSID: material, HTTPS: domain.HTTPSConfig{HTTPConfig: v.HTTPConfig, Protocols: v.Protocols, Ciphers: v.Algorithm, HSTS: v.HSTS, HSTSIncludeSubDomains: v.HSTSIncludeSubDomains, HTTP2: true, HTTP3: v.HTTP3}}
	if e = m.SaveHTTPS(s.Revision, site.ID, update); e != nil {
		return e
	}
	latest, _ := m.SSLList()
	overview, _ := m.Overview(r.Context())
	if overview.Runtime.Running {
		if e = m.Apply(r.Context(), latest.Revision); e != nil {
			return e
		}
	}
	writeJSON(w, 200, map[string]bool{"success": true})
	return nil
}
func findSSL(records []domain.SSLRecord, id uint) *domain.SSLRecord {
	for i := range records {
		if records[i].ID == id {
			return &records[i]
		}
	}
	return nil
}
