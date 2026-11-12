package domain

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Managed headers have dedicated GUI fields and cannot also appear as custom headers.
func ManagedResponseHeader(name string) bool {
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "access-control-") || slices.Contains([]string{"strict-transport-security", "alt-svc", "vary"}, name)
}

func ValidateResponseHeaders(site Site) error {
	if len(site.Headers) > 100 {
		return fmt.Errorf("自定义响应头不能超过 100 项")
	}
	scopes := map[string]bool{"server": true, "root": true}
	for _, code := range []string{"403", "404", "500", "502", "503", "504", "495", "496"} {
		scopes["error:"+code] = true
	}
	for _, proxy := range site.Proxies {
		scopes["proxy:"+proxy.ID] = true
	}
	for _, h := range site.Headers {
		if !scopes[h.Scope] || !headerName.MatchString(h.Name) || len(h.Name) > 128 || ManagedResponseHeader(h.Name) {
			return fmt.Errorf("响应头作用范围或名称无效；CORS、HSTS、Alt-Svc 和 Vary 由跨域及 HTTPS 模块维护，不能重复定义")
		}
		if len(h.Value) > 8192 || strings.ContainsFunc(h.Value, unicode.IsControl) {
			return fmt.Errorf("响应头值不能包含控制字符或超过 8192 字节")
		}
	}
	return nil
}
