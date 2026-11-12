package domain

import (
	"strings"
	"testing"
)

func websiteFixture() Site {
	s := Site{ID: NewID(), Name: "example", Kind: "static", Domains: []string{"example.test"}, Enabled: true}
	NormalizeSite(&s)
	return s
}
func TestWebsiteSettingsValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Site)
	}{
		{"last domain", func(s *Site) { s.Bindings = []DomainBinding{} }},
		{"port zero", func(s *Site) { s.Bindings[0].Port = 0 }},
		{"SSL on HTTP port", func(s *Site) { s.Bindings[0].SSL = true }},
		{"escape directory", func(s *Site) { s.RunDirectory = "/../other" }},
		{"hidden directory", func(s *Site) { s.RunDirectory = "/.private" }},
		{"php document", func(s *Site) { s.Index = []string{"index.PHP"} }},
		{"directive document", func(s *Site) { s.Index = []string{"index.html; return 200"} }},
		{"empty documents", func(s *Site) { s.Index = []string{} }},
		{"HTTP3 requires TLS13", func(s *Site) { s.HTTPS.HTTP3 = true; s.HTTPS.Protocols = []string{"TLSv1.2"} }},
		{"invalid TLS", func(s *Site) { s.HTTPS.Protocols = []string{"TLSv9"} }},
		{"cipher injection", func(s *Site) { s.HTTPS.Ciphers = "HIGH;return 200" }},
		{"HTTP bypass mTLS", func(s *Site) { s.ClientCAID = "ca"; s.HTTPS.HTTPConfig = "HTTPAlso" }},
		{"real IP header injection", func(s *Site) { s.RealIP = RealIPConfig{Enabled: true, From: []string{"127.0.0.1"}, Header: "X-IP;"} }},
		{"real IP CIDR", func(s *Site) {
			s.RealIP = RealIPConfig{Enabled: true, From: []string{"192.0.2.0/33"}, Header: "X-Real-IP"}
		}},
		{"CORS wildcard credentials", func(s *Site) { s.CORS = CORSConfig{Enabled: true, AllowOrigins: "*", AllowCredentials: true} }},
		{"CORS origin path", func(s *Site) { s.CORS = CORSConfig{Enabled: true, AllowOrigins: "https://example.test/path"} }},
		{"unbound redirect domain", func(s *Site) {
			s.Redirects = []RedirectRule{{ID: NewID(), Name: "bad", Enabled: true, Type: "domain", Domains: []string{"other.test"}, Redirect: 301, Target: "https://target.test"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := websiteFixture()
			test.change(&s)
			if ValidateSiteSettings(s) == nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
}
func TestProxyValidation(t *testing.T) {
	base := ProxyRule{ID: NewID(), Name: "api", Enabled: true, Match: "/api/", ProxyPass: "https://origin.test/prefix/", ProxyHost: "$host", ConnectTimeout: 60, ReadTimeout: 60, SendTimeout: 60}
	if err := ValidateProxy(base); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ProxyRule){func(p *ProxyRule) { p.ProxyPass = "http://a.test;return 200" }, func(p *ProxyRule) { p.ProxyPass = "http://user:secret@origin.test" }, func(p *ProxyRule) { p.ProxyHost = "$request_uri" }, func(p *ProxyRule) { p.Modifier = "~" }, func(p *ProxyRule) { p.Match = "/__resty_errors/404.html" }, func(p *ProxyRule) { p.Cache = true }, func(p *ProxyRule) { p.Replaces = []Replacement{{From: ""}} }} {
		p := base
		change(&p)
		if ValidateProxy(p) == nil {
			t.Fatalf("invalid proxy: %+v", p)
		}
	}
}
func TestLegacyWebsiteDefaults(t *testing.T) {
	s := State{Sites: []Site{{ID: NewID(), Kind: "proxy", Domains: []string{"legacy.test"}, Upstream: "https://origin.test"}}}
	NormalizeState(&s)
	site := s.Sites[0]
	if site.Proxies[0].Replaces == nil || site.Proxies[0].ServerCacheTime != 10 {
		t.Fatal("legacy proxy form defaults missing")
	}
	if site.RunDirectory != "/" || strings.Join(site.Index, ",") != "index.html" || site.HTTPS.HTTPConfig != "HTTPToHTTPS" || len(site.Bindings) != 1 || len(site.Proxies) != 1 || !site.Proxies[0].SSLVerify || !site.Proxies[0].SNI {
		t.Fatalf("lost legacy behavior: %+v", site)
	}
	site.Proxies = []ProxyRule{}
	NormalizeSite(&site)
	if len(site.Proxies) != 0 {
		t.Fatal("deleted legacy proxy recreated")
	}
}
