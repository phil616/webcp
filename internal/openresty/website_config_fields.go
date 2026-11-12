package openresty

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"web-panel/internal/domain"
	"web-panel/internal/onepanel/nginx/components"
)

func importRealIP(site *domain.Site, block components.IBlock) {
	var from []string
	for _, d := range configDirectives(block) {
		if d.GetName() == "set_real_ip_from" && len(configParams(d)) == 1 {
			from = append(from, configParams(d)[0])
		}
	}
	site.RealIP.Enabled = len(from) > 0
	if site.RealIP.Enabled {
		site.RealIP.From = from
		if header := configValues(block, "real_ip_header"); len(header) == 1 {
			site.RealIP.Header = header[0]
		}
	}
}

func importManagedHeaders(site *domain.Site, block components.IBlock, ssl bool) {
	hasOrigin := false
	for _, d := range configDirectives(block) {
		params := configParams(d)
		if d.GetName() == "add_header" && len(params) >= 2 && strings.EqualFold(params[0], "Access-Control-Allow-Origin") {
			hasOrigin = true
		}
	}
	if hasOrigin && configDirective(block, "root") != nil {
		site.CORS.AllowCredentials = false
		site.CORS.AllowMethods, site.CORS.AllowHeaders = "", ""
	}
	for _, d := range configDirectives(block) {
		params := configParams(d)
		if d.GetName() != "add_header" || len(params) < 2 {
			continue
		}
		switch strings.ToLower(params[0]) {
		case "strict-transport-security":
			if ssl {
				site.HTTPS.HSTS = true
				site.HTTPS.HSTSIncludeSubDomains = strings.Contains(params[1], "includeSubDomains")
			}
		case "access-control-allow-origin":
			hasOrigin = true
			if params[1] != corsVariable(site.ID) {
				site.CORS.AllowOrigins = params[1]
			}
		case "access-control-allow-methods":
			site.CORS.AllowMethods = params[1]
		case "access-control-allow-headers":
			site.CORS.AllowHeaders = params[1]
		case "access-control-allow-credentials":
			site.CORS.AllowCredentials = params[1] == "true"
		}
	}
	// HTTP redirect servers contain no website-level CORS settings.
	if configDirective(block, "root") != nil {
		site.CORS.Enabled = hasOrigin
	}
}

func importProxyConfig(proxy *domain.ProxyRule, block components.IBlock) error {
	if values := configValues(block, "proxy_pass"); len(values) == 1 {
		proxy.ProxyPass = values[0]
	}
	for _, d := range configDirectives(block) {
		params := configParams(d)
		if d.GetName() == "proxy_set_header" && len(params) == 2 && params[0] == "Host" {
			proxy.ProxyHost = params[1]
		}
	}
	for _, item := range []struct {
		name string
		out  *int
	}{{"proxy_connect_timeout", &proxy.ConnectTimeout}, {"proxy_read_timeout", &proxy.ReadTimeout}, {"proxy_send_timeout", &proxy.SendTimeout}} {
		if values := configValues(block, item.name); len(values) == 1 {
			value := strings.TrimSuffix(values[0], "s")
			number, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s 必须为 GUI 支持的整数秒数", item.name)
			}
			*item.out = number
		}
	}
	origin, _ := url.Parse(proxy.ProxyPass)
	if origin != nil && origin.Scheme == "https" {
		proxy.SNI = slices.Equal(configValues(block, "proxy_ssl_server_name"), []string{"on"})
		if values := configValues(block, "proxy_ssl_name"); len(values) == 1 {
			// Keep the GUI's automatic upstream-name setting when it is unchanged.
			if proxy.SSLName != "" || values[0] != origin.Hostname() {
				proxy.SSLName = values[0]
			}
		}
		proxy.SSLVerify = slices.Equal(configValues(block, "proxy_ssl_verify"), []string{"on"})
	}
	proxy.Cache = configDirective(block, "proxy_cache") != nil
	if values := configValues(block, "proxy_cache_valid"); len(values) == 3 {
		if err := importDuration(values[2], &proxy.ServerCacheTime, &proxy.ServerCacheUnit); err != nil {
			return err
		}
	}
	proxy.CacheTime = 0
	if values := configValues(block, "expires"); len(values) == 1 {
		if values[0] == "-1" {
			proxy.CacheTime = -1
		} else if err := importDuration(values[0], &proxy.CacheTime, &proxy.CacheUnit); err != nil {
			return err
		}
	}
	return nil
}

func importDuration(value string, count *int, unit *string) error {
	if len(value) < 2 || !strings.ContainsRune("smhd", rune(value[len(value)-1])) {
		return fmt.Errorf("缓存时间必须为 GUI 支持的整数和 s/m/h/d 单位")
	}
	number, err := strconv.Atoi(value[:len(value)-1])
	if err != nil {
		return fmt.Errorf("缓存时间格式无效")
	}
	*count, *unit = number, value[len(value)-1:]
	return nil
}
