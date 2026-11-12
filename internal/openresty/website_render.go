package openresty

import (
	"fmt"
	"slices"
	"strings"

	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
)

// quote protects nginx token boundaries while preserving PCRE anchors and module variables.
func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
func template(value, suffix string) string {
	q := quote(value)
	return strings.TrimSuffix(q, `"`) + suffix + `"`
}
func corsVariable(key string) string { return "$cors_" + strings.ReplaceAll(key, "-", "_") }
func renderCORSMap(b *strings.Builder, c domain.CORSConfig, key string) {
	origins := domain.OriginList(c.AllowOrigins)
	if !c.Enabled || len(origins) == 1 && (origins[0] == "*" || origins[0] == "$http_origin") {
		return
	}
	fmt.Fprintf(b, " map $http_origin %s { default '';\n", corsVariable(key))
	for _, origin := range domain.OriginList(c.AllowOrigins) {
		fmt.Fprintf(b, "  %s %s;\n", quote(origin), quote(origin))
	}
	b.WriteString(" }\n")
}
func renderHTTPGlobals(b *strings.Builder, state domain.State, internal map[int]int, web []WebsitePort, publicHTTPS int) {
	listeners, _ := domain.WebsiteListeners(state)
	keys := []int{}
	for port := range listeners {
		keys = append(keys, port)
	}
	slices.Sort(keys)
	udp := map[int]bool{}
	for _, p := range web {
		if p.UDP {
			udp[p.Port] = true
		}
	}
	for _, port := range keys {
		n := internal[port]
		if listeners[port] {
			fmt.Fprintf(b, " server { listen %d ssl default_server;", n)
			if udp[port] {
				fmt.Fprintf(b, " listen %d quic reuseport default_server;", n)
			}
			b.WriteString(" server_name _; ssl_reject_handshake on; }\n")
		} else {
			fmt.Fprintf(b, " server { listen %d default_server; server_name _; error_page 404 /__resty_errors/404.html; location = /__resty_errors/404.html { internal; root %s/root; try_files /default/404.html =404; } location ^~ /.well-known/acme-challenge/ { root %s/acme; } location / { return 404; } }\n", n, Mount, Mount)
		}
	}
	fmt.Fprintf(b, " map $http_host $resty_h3_port { default %d; ~:(?<resty_host_port>[0-9]+)$ $resty_host_port; }\n", publicHTTPS)
	for _, s := range state.Sites {
		if !s.Enabled {
			continue
		}
		renderCORSMap(b, s.CORS, s.ID)
		cache := false
		for _, p := range s.Proxies {
			if !p.Enabled {
				continue
			}
			renderCORSMap(b, p.CORS, s.ID+"_"+p.ID)
			cache = cache || p.Cache
		}
		if cache {
			fmt.Fprintf(b, " proxy_cache_path "+Mount+"/cache/%s levels=1:2 keys_zone=cache_%s:%dm max_size=%dm inactive=%dm use_temp_path=off;\n", s.ID, s.ID, s.ProxyCache.MemoryMB, s.ProxyCache.MaxSizeMB, s.ProxyCache.InactiveMinutes)
		}
	}

}
func sitePorts(s domain.Site) (plain, secure []int) {
	http, tls := map[int]bool{}, map[int]bool{}
	if s.TLSID != "" {
		tls[443] = true
	}
	for _, binding := range s.Bindings {
		if binding.Port == 443 && s.TLSID == "" {
			continue
		}
		if binding.SSL && s.TLSID != "" {
			tls[binding.Port] = true
		} else {
			http[binding.Port] = true
		}
	}
	for port := range http {
		plain = append(plain, port)
	}
	for port := range tls {
		secure = append(secure, port)
	}
	slices.Sort(plain)
	slices.Sort(secure)
	return
}
func renderWebsite(b *strings.Builder, s domain.Site, base string, internal map[int]int, publicHTTPS int, sitesRoot string) error {
	plain, secure := sitePorts(s)
	for _, group := range []struct {
		ports []int
		ssl   bool
	}{{plain, false}, {secure, true}} {
		if len(group.ports) == 0 {
			continue
		}
		b.WriteString(" server {\n")
		for _, port := range group.ports {
			fmt.Fprintf(b, "  listen %d", internal[port])
			if group.ssl {
				b.WriteString(" ssl")
			}
			b.WriteString(";\n")
			if group.ssl && s.HTTPS.HTTP3 {
				fmt.Fprintf(b, "  listen %d quic;\n", internal[port])
			}
		}
		fmt.Fprintf(b, "  server_name %s;\n", strings.Join(s.Domains, " "))
		fmt.Fprintf(b, "  location ^~ /.well-known/acme-challenge/ { root %s/acme; access_by_lua_block { return } }\n", Mount)
		if group.ssl {
			renderHTTPS(b, s, base)
		} else if s.TLSID != "" && s.HTTPS.HTTPConfig != "HTTPAlso" {
			if s.HTTPS.HTTPConfig == "HTTPSOnly" {
				b.WriteString("  location / { return 444; }\n")
			} else {
				port := ""
				if publicHTTPS != 443 {
					port = fmt.Sprintf(":%d", publicHTTPS)
				}
				fmt.Fprintf(b, "  location / { return 301 https://$host%s$request_uri; }\n", port)
			}
			b.WriteString(" }\n")
			continue
		}
		renderRealIP(b, s.RealIP)
		fmt.Fprintf(b, "  root %s/%s%s;\n  index %s;\n  disable_symlinks on;\n", sitesRoot, s.ID, strings.TrimSuffix(s.RunDirectory, "/"), strings.Join(s.Index, " "))
		renderHeaders(b, s, s.CORS, s.ID, group.ssl)
		// These phases apply to both static and proxy locations. content stays on static locations.
		for _, phase := range []string{"access", "header_filter", "body_filter", "log"} {
			if id := s.LuaBindings[phase]; id != "" {
				fmt.Fprintf(b, "  %s_by_lua_file %s/blocks/%s.lua;\n", phase, base, id)
			}
		}
		renderRedirects(b, s.Redirects)
		for _, code := range errorpages.Codes {
			certificateError := code == "495" || code == "496"
			if certificateError && (!group.ssl || s.ClientCAID == "") {
				continue
			}
			if code == "404" && has404Redirect(s.Redirects) {
				continue
			}
			status := code
			fallback := code
			if certificateError {
				status += " =403"
				fallback = "403"
			}
			fmt.Fprintf(b, "  error_page %s /__resty_errors/%s.html;\n  location = /__resty_errors/%s.html { internal; root %s/root; disable_symlinks on; try_files /sites/%s/%s.html /default/%s.html =%s;\n", status, code, code, Mount, s.ID, code, code, fallback)
			renderHeaders(b, s, s.CORS, s.ID, group.ssl)
			b.WriteString("  }\n")
		}
		// Rewrite and proxy locations cannot expose or execute PHP files.
		b.WriteString("  if ($uri ~* \\.(php[0-9]*|phtml|phar)(/|$)) { return 404; }\n  location ~ /\\. { deny all; }\n")
		rootProxy := false
		for _, p := range s.Proxies {
			if !p.Enabled {
				continue
			}
			if p.Match == "/" && p.Modifier == "" {
				rootProxy = true
			}
			renderProxy(b, s, p, group.ssl)
		}
		if !rootProxy {
			b.WriteString("  location / {\n")
			renderHeaders(b, s, s.CORS, s.ID, group.ssl)
			renderPreflight(b, s.CORS)
			if id := s.LuaBindings["content"]; id != "" && s.Kind == "static" {
				fmt.Fprintf(b, "   content_by_lua_file %s/blocks/%s.lua;\n", base, id)
			} else if s.Kind == "proxy" {
				b.WriteString("   return 404;\n")
			} else {
				b.WriteString("   try_files $uri $uri/ =404;\n")
			}
			b.WriteString("  }\n")
		}
		b.WriteString(" }\n")
	}
	return nil
}
