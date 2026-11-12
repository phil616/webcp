package domain

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var luaName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
var labelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

func ValidateDomains(domains []string) error {
	if len(domains) == 0 || len(domains) > 50 {
		return fmt.Errorf("需要 1–50 个域名或 IP")
	}
	seen := map[string]bool{}
	for _, domain := range domains {
		if seen[domain] {
			return fmt.Errorf("域名重复: %s", domain)
		}
		seen[domain] = true
		if net.ParseIP(domain) != nil {
			continue
		}
		name := strings.TrimPrefix(domain, "*.")
		if len(name) > 253 || len(name) == 0 {
			return fmt.Errorf("无效域名: %s", domain)
		}
		for _, label := range strings.Split(name, ".") {
			if !labelPattern.MatchString(label) {
				return fmt.Errorf("无效域名 %q，请使用小写 ASCII 或 punycode", domain)
			}
		}
	}
	return nil
}

func ValidatePages(pages map[string]string) error {
	for code, html := range pages {
		switch code {
		case "403", "404", "495", "496", "500", "502", "503", "504":
		default:
			return fmt.Errorf("不支持的错误码: %s", code)
		}
		if len(html) == 0 || len(html) > 256<<10 {
			return fmt.Errorf("错误页面大小必须为 1–256 KiB")
		}
	}
	return nil
}

func ValidateS3(s S3) error {
	if s.Endpoint == "" && s.Bucket == "" {
		return nil
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("S3 endpoint 必须是 http(s)://host[:port]")
	}
	if s.Bucket == "" || strings.ContainsAny(s.Bucket, "/\\\n\r ") || s.AccessKey == "" || s.SecretKey == "" {
		return fmt.Errorf("S3 存储桶和访问密钥不能为空")
	}
	if strings.Contains(s.Prefix, "..") || strings.ContainsAny(s.Prefix, "\\\r\n") || strings.HasPrefix(s.Prefix, "/") {
		return fmt.Errorf("无效 S3 前缀")
	}
	return nil
}

func Validate(s State) error {
	if err := ValidateOpenResty(s.OpenResty); err != nil {
		return err
	}
	if s.Revision == 0 {
		return fmt.Errorf("配置版本不能为零")
	}
	if s.Schema != SchemaVersion {
		return fmt.Errorf("不支持的配置版本: %d", s.Schema)
	}
	if len(s.Sites) > 1000 || len(s.Streams) > 1000 || len(s.Lua) > 1000 || len(s.Certificates) > 10000 {
		return fmt.Errorf("配置记录过多")
	}
	if err := ValidateS3(s.S3); err != nil {
		return err
	}
	ids := map[string]bool{}
	claim := func(id string) error {
		if !ValidID(id) || ids[id] {
			return fmt.Errorf("无效或重复 ID: %s", id)
		}
		ids[id] = true
		return nil
	}
	for i, proxy := range s.Streams {
		if err := claim(proxy.ID); err != nil {
			return err
		}
		if err := ValidateStream(proxy); err != nil {
			return err
		}
		for _, other := range s.Streams[:i] {
			if proxy.Name == other.Name {
				return fmt.Errorf("四层代理名称重复: %s", proxy.Name)
			}
			if proxy.Enabled && other.Enabled && proxy.Protocol == other.Protocol && ListenerOverlap(proxy.ListenAddress, proxy.ListenPort, other.ListenAddress, other.ListenPort) {
				return fmt.Errorf("四层代理监听端口冲突: %s 与 %s", proxy.Name, other.Name)
			}
		}
	}
	lua := map[string]Lua{}
	names := map[string]bool{}
	for _, l := range s.Lua {
		if err := claim(l.ID); err != nil {
			return err
		}
		if !luaName.MatchString(l.Name) || names[l.Name] {
			return fmt.Errorf("Lua 名称必须唯一，且只包含英文字母、数字和下划线")
		}
		names[l.Name] = true
		if l.Kind != "module" && l.Kind != "block" {
			return fmt.Errorf("Lua 类型必须为 module 或 block")
		}
		if strings.TrimSpace(l.Code) == "" || len(l.Code) > 256<<10 {
			return fmt.Errorf("Lua 代码必须为 1–256 KiB")
		}
		lua[l.ID] = l
	}
	certs := map[string]Certificate{}
	for _, c := range s.Certificates {
		if err := claim(c.ID); err != nil {
			return err
		}
		if c.Kind != "ca" && c.Kind != "server" {
			return fmt.Errorf("无效证书类型")
		}
		pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		if err != nil {
			return fmt.Errorf("证书 %s: %w", c.Name, err)
		}
		parsed, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return err
		}
		if parsed.SerialNumber.String() != c.Serial || !parsed.NotAfter.Equal(c.NotAfter) || !parsed.NotBefore.Equal(c.NotBefore) || parsed.IsCA != (c.Kind == "ca") {
			return fmt.Errorf("证书元数据不匹配: %s", c.Name)
		}
		certs[c.ID] = c
	}
	for _, c := range s.Certificates {
		if c.CAID == "" {
			continue
		}
		ca, ok := certs[c.CAID]
		if !ok || ca.Kind != "ca" {
			return fmt.Errorf("证书 CA 不存在")
		}
		block, _ := pem.Decode([]byte(c.CertPEM))
		parent, _ := pem.Decode([]byte(ca.CertPEM))
		leaf, _ := x509.ParseCertificate(block.Bytes)
		issuer, _ := x509.ParseCertificate(parent.Bytes)
		if err := leaf.CheckSignatureFrom(issuer); err != nil {
			return fmt.Errorf("证书签名与 CA 不匹配")
		}
	}
	domains := map[string]bool{}
	for _, site := range s.Sites {
		site = SiteDefaults(site)
		if err := ValidateSiteSettings(site); err != nil {
			return fmt.Errorf("网站 %s: %w", site.Name, err)
		}
		if err := claim(site.ID); err != nil {
			return err
		}
		if strings.TrimSpace(site.Name) == "" || utf8.RuneCountInString(site.Name) > 128 {
			return fmt.Errorf("网站名称必须为 1–128 字符")
		}
		if err := ValidateDomains(site.Domains); err != nil {
			return err
		}
		ports := map[int]bool{}
		for _, binding := range site.Bindings {
			ports[binding.Port] = true
		}
		if site.TLSID != "" {
			ports[443] = true
		}
		for _, d := range site.Domains {
			for port := range ports {
				key := fmt.Sprintf("%s:%d", d, port)
				if domains[key] {
					return fmt.Errorf("域名及端口已被其他网站使用: %s", key)
				}
				domains[key] = true
			}
		}
		if site.Kind != "static" && site.Kind != "proxy" {
			return fmt.Errorf("仅支持静态网站和单源反向代理")
		}
		if site.Kind == "proxy" && site.Upstream != "" {
			if err := ValidateProxyURL(site.Upstream); err != nil {
				return err
			}
		}
		for phase, id := range site.LuaBindings {
			switch phase {
			case "access", "content", "header_filter", "body_filter", "log":
			default:
				return fmt.Errorf("不支持的 Lua 阶段: %s", phase)
			}
			l, ok := lua[id]
			if !ok || l.Kind != "block" {
				return fmt.Errorf("Lua 代码块不存在或仍被引用")
			}
			if phase == "content" && site.Kind == "proxy" {
				return fmt.Errorf("反向代理不能同时绑定 content Lua")
			}
		}
		if site.TLSID != "" {
			c, ok := certs[site.TLSID]
			if !ok || c.Kind != "server" || c.RevokedAt != nil {
				return fmt.Errorf("需要有效的服务器证书")
			}
			if site.Enabled {
				block, _ := pem.Decode([]byte(c.CertPEM))
				parsed, _ := x509.ParseCertificate(block.Bytes)
				for _, d := range site.Domains {
					if strings.HasPrefix(d, "*.") {
						covered := false
						for _, san := range parsed.DNSNames {
							if strings.EqualFold(d, san) {
								covered = true
								break
							}
						}
						if !covered {
							return fmt.Errorf("证书不覆盖通配域名: %s", d)
						}
					} else if err := parsed.VerifyHostname(d); err != nil {
						return fmt.Errorf("证书不覆盖域名: %s", d)
					}
				}
			}
		}
	}
	if err := ValidateMTLS(s); err != nil {
		return err
	}
	if _, err := WebsiteListeners(s); err != nil {
		return err
	}

	return nil
}
