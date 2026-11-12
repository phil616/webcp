package openresty

import (
	"reflect"
	"strings"
	"testing"

	"web-panel/internal/domain"
)

func configSite() domain.Site {
	return domain.SiteDefaults(domain.Site{ID: domain.NewID(), Name: "config", Kind: "static", Enabled: true, Domains: []string{"config.test"}})
}

func TestWebsiteConfigRoundTrip(t *testing.T) {
	for _, kind := range []string{"static", "proxy", "https", "mtls"} {
		t.Run(kind, func(t *testing.T) {
			site := configSite()
			if kind == "proxy" {
				site.Kind, site.Upstream, site.Proxies = "proxy", "https://origin.test", nil
				domain.NormalizeSite(&site)
				site.Proxies[0].Cache = true
				site.Proxies[0].CacheTime = 2
				site.Proxies[0].Replaces = []domain.Replacement{{From: "old", To: "new"}}
			}
			if kind == "https" || kind == "mtls" {
				site.TLSID = domain.NewID()
				site.HTTPS.HTTPConfig = "HTTPAlso"
				site.HTTPS.HSTS, site.HTTPS.HSTSIncludeSubDomains, site.HTTPS.HTTP3 = true, true, true
			}
			if kind == "mtls" {
				site.ClientCAID = domain.NewID()
				site.HTTPS.HTTPConfig = "HTTPToHTTPS"
			}
			site.CORS = domain.DefaultCORS()
			site.CORS.Enabled, site.CORS.AllowOrigins = true, "https://one.test,https://two.test"
			site.RealIP = domain.RealIPConfig{Enabled: true, From: []string{"127.0.0.1", "10.0.0.0/8"}, Header: "X-Real-IP"}
			site.Headers = []domain.ResponseHeader{{Scope: "server", Name: "X-First", Value: `a "quote" and \ path`, Always: true}, {Scope: "root", TLS: true, Name: "X-TLS", Value: "$request_id"}}
			text, err := WebsiteConfig(site, 8443)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ImportWebsiteConfig(site, text, 8443)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, site) {
				t.Fatalf("round trip changed GUI state:\ngot  %#v\nwant %#v", got, site)
			}
		})
	}
}

func TestWebsiteConfigImportsAndRejectsLossyChanges(t *testing.T) {
	site := configSite()
	text, _ := WebsiteConfig(site, 443)
	edited := strings.Replace(text, "index index.html;", "index home.html index.html;", 1)
	edited = strings.Replace(edited, "disable_symlinks on;", "disable_symlinks off;", 1)
	edited = strings.Replace(edited, "  location ~ /\\. { deny all; }\n", "", 1)
	edited = strings.Replace(edited, "\n }\n", "\n add_header X-Test added always;\n }\n", 1)
	got, err := ImportWebsiteConfig(site, edited, 443)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Security.AllowHiddenFiles || !got.Security.FollowSymlinks || got.Index[0] != "home.html" || len(got.Headers) != 1 || got.Headers[0].Value != "added" {
		t.Fatalf("edits were not mapped: %#v", got)
	}
	for name, invalid := range map[string]string{
		"valid but unsupported": strings.Replace(text, "server {", "server { add_trailer X-Test added always;", 1),
		"include":               strings.Replace(text, "server {", "server { include /tmp/example.conf;", 1),
		"literal dollar":        strings.Replace(text, "server {", `server { add_header X-Test "\$host";`, 1),
		"PHP":                   strings.Replace(text, "return 404;", "return 200;", 1),
		"Lua":                   strings.Replace(text, "access_by_lua_block { return }", "access_by_lua_block { ngx.exit(200) }", 1),
		"duplicate":             strings.Replace(text, "index index.html;", "index index.html; index home.html;", 1),
		"missing block end":     strings.TrimSuffix(text, " }\n"),
		"unexpected block end":  text + "}\nserver { listen 8000; }",
		"partial parse":         text + "garbage",
		"extra block":           text + "server { listen 8000; }",
		"trailing semicolon":    text + ";",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ImportWebsiteConfig(site, invalid, 443); err == nil {
				t.Fatal("accepted an unrepresentable configuration")
			}
		})
	}
}

func TestWebsiteConfigSharedAndLocationSettings(t *testing.T) {
	site := configSite()
	site.TLSID, site.HTTPS.HTTPConfig = domain.NewID(), "HTTPAlso"
	text, _ := WebsiteConfig(site, 443)
	if _, err := ImportWebsiteConfig(site, strings.Replace(text, "index index.html;", "index home.html;", 1), 443); err == nil {
		t.Fatal("accepted different indexes in HTTP and HTTPS despite a shared GUI field")
	}
	text = strings.Replace(text, "  location / {\n", "  location / {\n add_header X-Location scoped always;\n", 1)
	got, err := ImportWebsiteConfig(site, text, 443)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Headers) != 1 || got.Headers[0].TLS || got.Headers[0].Scope != "root" {
		t.Fatalf("lost header scope: %+v", got.Headers)
	}
}
