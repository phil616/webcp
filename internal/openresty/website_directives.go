package openresty

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"web-panel/internal/domain"
)

func renderHTTPS(b *strings.Builder, s domain.Site, base string) {
	fmt.Fprintf(b, "  ssl_protocols %s;\n  ssl_certificate %s/certs/%s.pem;\n  ssl_certificate_key %s/certs/%s.key;\n", strings.Join(s.HTTPS.Protocols, " "), base, s.TLSID, base, s.TLSID)
	if s.HTTPS.Ciphers != "" {
		fmt.Fprintf(b, "  ssl_ciphers %s;\n", quote(s.HTTPS.Ciphers))
	}
	if s.HTTPS.HTTP2 {
		b.WriteString("  http2 on;\n")
	}
	if s.ClientCAID != "" {
		fmt.Fprintf(b, "  ssl_client_certificate %s/mtls/%s.pem;\n  ssl_crl %s/mtls/%s.crl;\n  ssl_verify_client on;\n  ssl_verify_depth 2;\n  ssl_session_cache off;\n  ssl_session_tickets off;\n", base, s.ClientCAID, base, s.ClientCAID)
	}
}
func renderRealIP(b *strings.Builder, c domain.RealIPConfig) {
	if !c.Enabled {
		return
	}
	b.WriteString("  real_ip_recursive on;\n")
	for _, ip := range c.From {
		fmt.Fprintf(b, "  set_real_ip_from %s;\n", ip)
	}
	fmt.Fprintf(b, "  real_ip_header %s;\n", c.Header)
}
func renderResponseHeaders(b *strings.Builder, s domain.Site, scope string, ssl bool) {
	for _, header := range s.Headers {
		if header.Scope != scope || header.TLS != ssl {
			continue
		}
		fmt.Fprintf(b, "  add_header %s %s", header.Name, quote(header.Value))
		if header.Always {
			b.WriteString(" always")
		}
		b.WriteString(";\n")
	}
}
func renderHeaders(b *strings.Builder, s domain.Site, c domain.CORSConfig, key, scope string, ssl bool) {
	renderResponseHeaders(b, s, scope, ssl)
	if ssl && s.HTTPS.HSTS {
		value := "max-age=31536000"
		if s.HTTPS.HSTSIncludeSubDomains {
			value += "; includeSubDomains"
		}
		fmt.Fprintf(b, "  add_header Strict-Transport-Security %s always;\n", quote(value))
	}
	if ssl && s.HTTPS.HTTP3 {
		b.WriteString("  add_header Alt-Svc 'h3=\":$resty_h3_port\"; ma=86400' always;\n")
	}
	if !c.Enabled {
		return
	}
	origin := corsVariable(key)
	origins := domain.OriginList(c.AllowOrigins)
	if len(origins) == 1 && origins[0] == "*" {
		origin = quote("*")
	} else if len(origins) == 1 && origins[0] == "$http_origin" {
		origin = "$http_origin"
	}
	fmt.Fprintf(b, "  add_header Access-Control-Allow-Origin %s always;\n", origin)
	if c.AllowMethods != "" {
		fmt.Fprintf(b, "  add_header Access-Control-Allow-Methods %s always;\n", quote(c.AllowMethods))
	}
	if c.AllowHeaders != "" {
		fmt.Fprintf(b, "  add_header Access-Control-Allow-Headers %s always;\n", quote(c.AllowHeaders))
	}
	if c.AllowCredentials {
		b.WriteString("  add_header Access-Control-Allow-Credentials true always;\n")
	}
	b.WriteString("  add_header Vary Origin always;\n")
}
func renderPreflight(b *strings.Builder, c domain.CORSConfig) {
	if c.Enabled && c.Preflight {
		b.WriteString("   add_header Access-Control-Max-Age $resty_preflight_age always;\n   if ($request_method = OPTIONS) { return 204; }\n")
	}
}
func has404Redirect(rules []domain.RedirectRule) bool {
	return slices.ContainsFunc(rules, func(r domain.RedirectRule) bool { return r.Enabled && r.Type == "404" })
}
func renderRedirects(b *strings.Builder, rules []domain.RedirectRule) {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		switch r.Type {
		case "domain":
			for _, d := range r.Domains {
				pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(d), `\*`, "[^.]+") + "$"
				target := quote(r.Target)
				if r.KeepPath {
					target = template(r.Target, "$request_uri")
				}
				fmt.Fprintf(b, "  if ($host ~* %s) { return %d %s; }\n", quote(pattern), r.Redirect, target)
			}
		case "path":
			target := template(r.Target, "?")
			if r.KeepPath {
				target = template(r.Target, "$1")
			}
			way := "permanent"
			if r.Redirect == 302 {
				way = "redirect"
			}
			fmt.Fprintf(b, "  rewrite %s %s %s;\n", quote("^"+r.Path+"(.*)"), target, way)
		case "404":
			target := r.Target
			if r.RedirectRoot {
				target = "/"
			}
			fmt.Fprintf(b, "  error_page 404 = @resty_notfound;\n  location @resty_notfound { return %d %s; }\n", r.Redirect, quote(target))
		}
	}
}
func renderProxy(b *strings.Builder, s domain.Site, p domain.ProxyRule, ssl bool) {
	fmt.Fprintf(b, "  location %s %s {\n", p.Modifier, quote(p.Match))
	b.WriteString("   proxy_http_version 1.1;\n   proxy_intercept_errors on;\n   proxy_set_header X-Real-IP $remote_addr;\n   proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n   proxy_set_header X-Forwarded-Proto $scheme;\n   proxy_set_header Upgrade $http_upgrade;\n   proxy_set_header Connection $connection_upgrade;\n")
	host := quote(p.ProxyHost)
	if slices.Contains([]string{"$host", "$http_host", "$proxy_host"}, p.ProxyHost) {
		host = p.ProxyHost
	}
	fmt.Fprintf(b, "   proxy_set_header Host %s;\n", host)
	fmt.Fprintf(b, "   proxy_connect_timeout %ds;\n   proxy_read_timeout %ds;\n   proxy_send_timeout %ds;\n", p.ConnectTimeout, p.ReadTimeout, p.SendTimeout)
	u, _ := url.Parse(p.ProxyPass)
	if u.Scheme == "https" {
		if p.SNI {
			b.WriteString("   proxy_ssl_server_name on;\n")
			name := p.SSLName
			if name == "" {
				name = u.Hostname()
			}
			fmt.Fprintf(b, "   proxy_ssl_name %s;\n", quote(name))
		}
		if p.SSLVerify {
			b.WriteString("   proxy_ssl_verify on;\n   proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;\n   proxy_ssl_verify_depth 4;\n")
		}
	}
	c, key := s.CORS, s.ID
	if p.CORS.Enabled {
		c, key = p.CORS, s.ID+"_"+p.ID
	}
	renderHeaders(b, s, c, key, "proxy:"+p.ID, ssl)
	renderPreflight(b, c)
	if c.Enabled {
		for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Allow-Credentials", "Access-Control-Expose-Headers", "Access-Control-Max-Age"} {
			fmt.Fprintf(b, "   proxy_hide_header %s;\n", h)
		}
	}
	if p.Cache {
		fmt.Fprintf(b, "   proxy_cache cache_%s;\n   proxy_cache_key \"%s:$scheme:$request_method:$host:$request_uri\";\n   proxy_cache_valid 200 304 %d%s;\n   proxy_cache_bypass $http_authorization $http_cookie $http_upgrade;\n   proxy_no_cache $http_authorization $http_cookie $http_upgrade $upstream_http_set_cookie;\n", s.ID, p.ID, p.ServerCacheTime, p.ServerCacheUnit)
	}
	if p.CacheTime > 0 {
		fmt.Fprintf(b, "   expires %d%s;\n", p.CacheTime, p.CacheUnit)
	} else if p.CacheTime < 0 {
		b.WriteString("   expires -1;\n")
	}
	if len(p.Replaces) > 0 {
		b.WriteString("   proxy_set_header Accept-Encoding '';\n   sub_filter_once off;\n   sub_filter_types *;\n")
		for _, r := range p.Replaces {
			fmt.Fprintf(b, "   sub_filter %s %s;\n", quote(r.From), quote(r.To))
		}
	}
	fmt.Fprintf(b, "   proxy_pass %s;\n  }\n", quote(p.ProxyPass))
}

func ProxyFragment(site domain.Site, p domain.ProxyRule) string {
	site = domain.SiteDefaults(site)
	var out strings.Builder
	renderProxy(&out, site, p, site.TLSID != "")
	return out.String()
}
func RedirectFragment(r domain.RedirectRule) string {
	var out strings.Builder
	renderRedirects(&out, []domain.RedirectRule{r})
	return out.String()
}
