package service

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"web-panel/internal/onepanel/dto"
)

// 1Panel gets this transport from panel-wide settings. The standalone service
// uses standard process proxy variables instead of introducing a settings panel.
func sslProxy(enabled bool) (*dto.SystemProxy, error) {
	if !enabled {
		return nil, nil
	}
	var raw string
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if raw = os.Getenv(name); raw != "" {
			break
		}
	}
	if raw == "" {
		return nil, fmt.Errorf("启用 ACME 代理前请配置 HTTPS_PROXY 或 HTTP_PROXY")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" {
		return nil, fmt.Errorf("ACME 代理地址无效")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
		if u.Scheme == "socks5" {
			port = "1080"
		}
	}
	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	p := &dto.SystemProxy{Type: u.Scheme, URL: host, Port: port}
	if u.User != nil {
		p.User = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	return p, nil
}
