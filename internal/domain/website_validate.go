package domain

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var headerName = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_\x60|~-]+$`)
var safeDocument = regexp.MustCompile(`^[A-Za-z0-9_.@/-]+$`)
var phpFile = regexp.MustCompile(`(?i)\.(?:php[0-9]*|phtml|phar)(?:[./]|$)`)

func ValidateRunDirectory(dir string) error {
	if dir == "/" {
		return nil
	}
	if !strings.HasPrefix(dir, "/") || path.Clean(dir) != dir || !safeDocument.MatchString(dir) {
		return fmt.Errorf("运行目录必须是网站根目录下的规范子目录")
	}
	for _, part := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("运行目录不允许隐藏目录或路径穿越")
		}
	}
	return nil
}
func ValidateCORS(c CORSConfig) error {
	if !c.Enabled {
		return nil
	}
	origins := OriginList(c.AllowOrigins)
	if len(origins) == 0 || len(origins) > 100 {
		return fmt.Errorf("请填写允许的来源")
	}
	for _, origin := range origins {
		if origin == "*" || origin == "$http_origin" {
			if len(origins) != 1 {
				return fmt.Errorf("通配或动态来源不能与其他来源混用")
			}
			if origin == "*" && c.AllowCredentials {
				return fmt.Errorf("携带凭据时不能使用 * 来源，请指定来源或 $http_origin")
			}
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || unsafeValue(origin) {
			return fmt.Errorf("跨域来源必须为 http(s)://host[:port]，不含路径")
		}
	}
	for _, list := range []string{c.AllowMethods, c.AllowHeaders} {
		if len(list) > 2048 {
			return fmt.Errorf("跨域字段过长")
		}
		for _, token := range strings.Split(list, ",") {
			if token != "" && !headerName.MatchString(strings.TrimSpace(token)) {
				return fmt.Errorf("跨域方法或请求头格式无效")
			}
		}
	}
	return nil
}
func OriginList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' || r == ' ' })
}
func unsafeValue(s string) bool { return strings.ContainsAny(s, ";{}$\\\"' \t\r\n\x00") }
func ValidateProxyURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || unsafeValue(value) {
		return fmt.Errorf("代理地址必须是单个 HTTP(S) URL")
	}
	if err = ValidateDomains([]string{strings.ToLower(u.Hostname())}); err != nil || strings.HasPrefix(u.Hostname(), "*.") {
		return fmt.Errorf("代理目标域名或 IP 无效")
	}
	if u.Port() != "" {
		n, e := strconv.Atoi(u.Port())
		if e != nil || n < 1 || n > 65535 {
			return fmt.Errorf("代理目标端口无效")
		}
	}
	return nil
}
func durationValid(n int, unit string) bool {
	return n > 0 && n <= 65535 && slices.Contains([]string{"s", "m", "h", "d"}, unit)
}
func ValidateProxy(p ProxyRule) error {
	if !ValidID(p.ID) || strings.TrimSpace(p.Name) == "" || len(p.Name) > 128 {
		return fmt.Errorf("代理规则 ID 或名称无效")
	}
	if !slices.Contains([]string{"", "=", "^~", "~", "~*"}, p.Modifier) || p.Match == "" || len(p.Match) > 1024 || strings.ContainsAny(p.Match, "\x00\r\n") {
		return fmt.Errorf("代理匹配规则无效")
	}
	if (p.Modifier == "" || p.Modifier == "=" || p.Modifier == "^~") && (!strings.HasPrefix(p.Match, "/") || strings.HasPrefix(p.Match, "/__resty_errors")) {
		return fmt.Errorf("代理匹配路径必须以 / 开头，且不能使用保留路径")
	}
	if err := ValidateProxyURL(p.ProxyPass); err != nil {
		return err
	}
	u, _ := url.Parse(p.ProxyPass)
	if (p.Modifier == "~" || p.Modifier == "~*") && (u.Path != "" || u.RawQuery != "") {
		return fmt.Errorf("正则代理的目标不能包含 URI 路径或查询")
	}
	if !slices.Contains([]string{"$host", "$http_host", "$proxy_host"}, p.ProxyHost) {
		if p.ProxyHost == "" || unsafeValue(p.ProxyHost) || strings.ContainsAny(p.ProxyHost, "/?#") {
			return fmt.Errorf("发送域名必须为域名/IP[:端口] 或 $host、$http_host、$proxy_host")
		}
		host := p.ProxyHost
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		if err := ValidateDomains([]string{host}); err != nil {
			return fmt.Errorf("发送域名无效")
		}
	}
	if p.SNI && p.SSLName != "" {
		if err := ValidateDomains([]string{p.SSLName}); err != nil || strings.Contains(p.SSLName, "*") {
			return fmt.Errorf("SNI 域名无效")
		}
	}
	for _, n := range []int{p.ConnectTimeout, p.ReadTimeout, p.SendTimeout} {
		if n < 1 || n > 3600 {
			return fmt.Errorf("代理超时需为 1–3600 秒")
		}
	}
	if p.Cache && !durationValid(p.ServerCacheTime, p.ServerCacheUnit) {
		return fmt.Errorf("服务端缓存时间无效")
	}
	if p.CacheTime < -1 || (p.CacheTime > 0 && !durationValid(p.CacheTime, p.CacheUnit)) {
		return fmt.Errorf("浏览器缓存时间无效")
	}
	if len(p.Replaces) > 50 {
		return fmt.Errorf("内容替换规则过多")
	}
	seen := map[string]bool{}
	for _, r := range p.Replaces {
		if r.From == "" || len(r.From) > 4096 || len(r.To) > 4096 || strings.ContainsAny(r.From+r.To, "\x00\r\n") || seen[r.From] {
			return fmt.Errorf("内容替换原文不能为空或重复，且不能包含换行")
		}
		seen[r.From] = true
	}
	return ValidateCORS(p.CORS)
}
func ValidateSiteSettings(site Site) error {
	s := SiteDefaults(site)
	if s.ProxyCache.MemoryMB < 1 || s.ProxyCache.MemoryMB > 1024 || s.ProxyCache.MaxSizeMB < 1 || s.ProxyCache.MaxSizeMB > 1048576 || s.ProxyCache.InactiveMinutes < 1 || s.ProxyCache.InactiveMinutes > 1051200 {
		return fmt.Errorf("代理缓存容量或有效期无效")
	}
	if len(s.Bindings) < 1 || len(s.Bindings) > 100 {
		return fmt.Errorf("网站需要 1–100 条域名绑定，不能删除最后一条")
	}
	seen := map[string]bool{}
	for _, b := range s.Bindings {
		if err := ValidateDomains([]string{b.Domain}); err != nil {
			return err
		}
		key := fmt.Sprintf("%s:%d", b.Domain, b.Port)
		if seen[key] || b.Port < 1 || b.Port > 65535 || b.Port == 80 && b.SSL || b.Port == 443 && !b.SSL {
			return fmt.Errorf("域名端口重复或无效；80 为 HTTP，443 为 HTTPS")
		}
		seen[key] = true
	}
	if err := ValidateRunDirectory(s.RunDirectory); err != nil {
		return err
	}
	if len(s.Index) < 1 || len(s.Index) > 50 {
		return fmt.Errorf("默认文档需要 1–50 项")
	}
	seen = map[string]bool{}
	for _, doc := range s.Index {
		if len(doc) > 255 || !safeDocument.MatchString(doc) || strings.HasPrefix(doc, "/") || path.Clean(doc) != doc || strings.Contains(doc, "..") || phpFile.MatchString(doc) || seen[doc] {
			return fmt.Errorf("默认文档必须是非 PHP 的相对文件路径，不能重复或包含特殊字符")
		}
		seen[doc] = true
	}
	if !slices.Contains([]string{"HTTPToHTTPS", "HTTPAlso", "HTTPSOnly"}, s.HTTPS.HTTPConfig) {
		return fmt.Errorf("HTTPS 访问模式无效")
	}
	if s.ClientCAID != "" && s.HTTPS.HTTPConfig == "HTTPAlso" {
		return fmt.Errorf("启用 mTLS 时不能允许明文 HTTP")
	}
	if len(s.HTTPS.Protocols) == 0 {
		return fmt.Errorf("请选择 TLS 协议")
	}
	for _, p := range s.HTTPS.Protocols {
		if p != "TLSv1" && p != "TLSv1.1" && p != "TLSv1.2" && p != "TLSv1.3" {
			return fmt.Errorf("无效 TLS 协议")
		}
	}
	if s.HTTPS.HTTP3 && !slices.Contains(s.HTTPS.Protocols, "TLSv1.3") {
		return fmt.Errorf("HTTP/3 需要启用 TLSv1.3")
	}
	if len(s.HTTPS.Ciphers) > 2048 || unsafeValue(s.HTTPS.Ciphers) {
		return fmt.Errorf("TLS 加密套件格式无效")
	}
	if err := ValidateCORS(s.CORS); err != nil {
		return err
	}
	if s.RealIP.Enabled {
		if len(s.RealIP.From) < 1 || len(s.RealIP.From) > 1000 || !headerName.MatchString(s.RealIP.Header) || strings.EqualFold(s.RealIP.Header, "proxy_protocol") {
			return fmt.Errorf("真实 IP 需要信任来源和有效请求头")
		}
		for _, ip := range s.RealIP.From {
			if net.ParseIP(ip) == nil {
				if _, _, err := net.ParseCIDR(ip); err != nil {
					return fmt.Errorf("真实 IP 信任来源必须是 IP 或 CIDR")
				}
			}
		}
	}
	seen = map[string]bool{}
	locations := map[string]bool{}
	if len(s.Proxies) > 100 || len(s.Redirects) > 100 {
		return fmt.Errorf("网站代理或重定向规则过多")
	}
	for _, p := range s.Proxies {
		if err := ValidateProxy(p); err != nil {
			return err
		}
		if seen[p.ID] || seen["name:"+p.Name] {
			return fmt.Errorf("代理名称或 ID 重复")
		}
		seen[p.ID] = true
		seen["name:"+p.Name] = true
		modifier := p.Modifier
		if modifier == "^~" {
			modifier = ""
		}
		key := modifier + " " + p.Match
		if p.Enabled && locations[key] {
			return fmt.Errorf("代理匹配路径重复")
		}
		if p.Enabled {
			locations[key] = true
		}
	}
	seen = map[string]bool{}
	sources := map[string]bool{}
	for _, r := range s.Redirects {
		if !ValidID(r.ID) || r.Name == "" || len(r.Name) > 128 || seen[r.ID] || seen["name:"+r.Name] {
			return fmt.Errorf("重定向名称或 ID 无效或重复")
		}
		seen[r.ID] = true
		seen["name:"+r.Name] = true
		if r.Redirect != 301 && r.Redirect != 302 {
			return fmt.Errorf("重定向方式只能是 301 或 302")
		}
		if !(r.Type == "404" && r.RedirectRoot) {
			if err := ValidateProxyURL(r.Target); err != nil {
				return fmt.Errorf("重定向目标无效: %w", err)
			}
		}
		if r.KeepPath && r.Type != "404" {
			u, _ := url.Parse(r.Target)
			if u.RawQuery != "" {
				return fmt.Errorf("保留路径时，重定向目标不能包含查询参数")
			}
		}
		switch r.Type {
		case "domain":
			if len(r.Domains) == 0 {
				return fmt.Errorf("请选择重定向来源域名")
			}
			for _, d := range r.Domains {
				if !slices.Contains(s.Domains, d) {
					return fmt.Errorf("重定向来源域名未绑定网站")
				}
				if sources[d] {
					return fmt.Errorf("同一来源域名只能有一条重定向")
				}
				sources[d] = true
			}
		case "path":
			if !strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, "\x00\r\n") || len(r.Path) > 1024 {
				return fmt.Errorf("重定向路径必须以 / 开头")
			}
			if sources["path:"+r.Path] {
				return fmt.Errorf("重定向路径重复")
			}
			sources["path:"+r.Path] = true
		case "404":
			if sources["404"] {
				return fmt.Errorf("只能设置一条 404 重定向")
			}
			sources["404"] = true
		default:
			return fmt.Errorf("重定向类型必须为域名、路径或 404")
		}
	}
	return nil
}

// Website listens share socket-wide TLS semantics, including implicit HTTPS 443.
func WebsiteListeners(state State) (map[int]bool, error) {
	ports := map[int]bool{80: false, 443: true}
	for _, raw := range state.Sites {
		s := SiteDefaults(raw)
		if !s.Enabled {
			continue
		}
		for _, b := range s.Bindings {
			tls := b.SSL && s.TLSID != ""
			if b.Port == 443 && s.TLSID == "" {
				continue
			}
			if prior, ok := ports[b.Port]; ok && prior != tls {
				return nil, fmt.Errorf("端口 %d 的 HTTP 与 HTTPS 监听冲突", b.Port)
			}
			ports[b.Port] = tls
		}
	}
	if len(ports) > 1000 {
		return nil, fmt.Errorf("网站监听端口过多")
	}
	return ports, nil
}
