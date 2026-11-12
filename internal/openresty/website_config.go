package openresty

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"web-panel/internal/domain"
	"web-panel/internal/onepanel/nginx/components"
	"web-panel/internal/onepanel/nginx/parser"
)

const MaxWebsiteConfigSize = 256 << 10

// WebsiteConfig renders an editable draft with public website ports and stable resource
// paths. Deployment resolves these to container ports and immutable generation paths.
func WebsiteConfig(site domain.Site, httpsPort int) (string, error) {
	site = domain.SiteDefaults(site)
	ports := map[int]int{80: 80, 443: 443}
	for _, binding := range site.Bindings {
		ports[binding.Port] = binding.Port
	}
	if httpsPort == 0 {
		httpsPort = 443
	}
	var out strings.Builder
	err := renderWebsite(&out, site, Mount+"/current", ports, httpsPort, Mount+"/sites")
	return out.String(), err
}

func parseWebsiteConfig(text string) (config *components.Config, err error) {
	if len(text) == 0 || len(text) > MaxWebsiteConfigSize || strings.ContainsRune(text, 0) {
		return nil, fmt.Errorf("网站配置不能为空、包含 NUL 或超过 256 KiB")
	}
	defer func() {
		if value := recover(); value != nil {
			config = nil
			err = fmt.Errorf("1Panel 解析器无法解析网站配置: %v", value)
		}
	}()
	return parser.NewStringParser(text).Parse()
}

func configDirectives(block components.IBlock) []components.IDirective {
	var result []components.IDirective
	for _, directive := range block.GetDirectives() {
		if directive.GetName() != "" { // Comments carry no configuration semantics.
			result = append(result, directive)
		}
	}
	return result
}

// nginxValue only decodes quoting emitted by our renderer. Other escape forms are
// retained and will be rejected unless they survive the complete round trip.
func nginxValue(value string) string {
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		q := string(value[0])
		return strings.NewReplacer(`\`+q, q, `\\`, `\`).Replace(value[1 : len(value)-1])
	}
	return value
}

func configParams(d components.IDirective) []string {
	params := slices.Clone(d.GetParameters())
	for i := range params {
		params[i] = nginxValue(params[i])
	}
	return params
}

// Comparing escaped tokens as decoded strings can change literal dollars into
// variables. Ignore only surrounding quotes; retain every escape in the check.
func comparableParams(d components.IDirective) []string {
	params := slices.Clone(d.GetParameters())
	for i, value := range params {
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			params[i] = value[1 : len(value)-1]
		}
	}
	return params
}

func configDirective(block components.IBlock, name string) components.IDirective {
	for _, directive := range configDirectives(block) {
		if directive.GetName() == name {
			return directive
		}
	}
	return nil
}

func configValues(block components.IBlock, name string) []string {
	if d := configDirective(block, name); d != nil {
		return configParams(d)
	}
	return nil
}

func configMismatch(actual, expected components.IBlock, scope string) error {
	a, b := comparableDirectives(actual), comparableDirectives(expected)
	for i, d := range a {
		if i >= len(b) || d.GetName() != b[i].GetName() || !slices.Equal(comparableParams(d), comparableParams(b[i])) || (d.GetBlock() == nil) != (b[i].GetBlock() == nil) {
			return fmt.Errorf("第 %d 行 %s/%s 无法无损映射到 GUI；不支持的指令、重复指令或相互矛盾的设置均不会保存", d.GetLine(), scope, d.GetName())
		}
		if d.GetBlock() != nil {
			if d.GetBlock().GetCodeBlock() != b[i].GetBlock().GetCodeBlock() {
				return fmt.Errorf("第 %d 行 Lua 内容请通过 Lua 模块管理", d.GetLine())
			}
			if err := configMismatch(d.GetBlock(), b[i].GetBlock(), scope+"/"+d.GetName()); err != nil {
				return err
			}
		}
	}
	if len(a) != len(b) {
		return fmt.Errorf("%s 缺少 GUI 所需的 %s 指令，无法无损映射", scope, b[len(a)].GetName())
	}
	return nil
}

// Header declarations may move around other directives without changing their
// meaning. Preserve order within custom and managed headers (including duplicates).
func comparableDirectives(block components.IBlock) []components.IDirective {
	var others, managed, custom []components.IDirective
	for _, d := range configDirectives(block) {
		params := configParams(d)
		if d.GetName() != "add_header" || len(params) < 2 {
			others = append(others, d)
		} else if domain.ManagedResponseHeader(params[0]) {
			managed = append(managed, d)
		} else {
			custom = append(custom, d)
		}
	}
	return append(append(others, managed...), custom...)
}

// ImportWebsiteConfig accepts only configurations fully represented by the typed GUI
// model. Re-rendering and comparing the entire AST prevents partial or silent imports.
func ImportWebsiteConfig(current domain.Site, text string, httpsPort int) (domain.Site, error) {
	encoded, err := json.Marshal(current)
	if err != nil {
		return current, err
	}
	var site domain.Site
	if err = json.Unmarshal(encoded, &site); err != nil {
		return current, err
	}
	domain.NormalizeSite(&site)
	parsed, err := parseWebsiteConfig(text)
	if err != nil {
		return current, err
	}
	site.Headers = []domain.ResponseHeader{}
	visible := map[string]bool{}
	for _, server := range configDirectives(parsed) {
		if server.GetName() != "server" || server.GetBlock() == nil {
			return current, fmt.Errorf("第 %d 行仅允许当前网站的 server 配置，不支持全局指令或额外上下文", server.GetLine())
		}
		if err = importWebsiteServer(&site, server.GetBlock(), visible); err != nil {
			return current, err
		}
	}
	// Disabled protocols/rules can have GUI settings absent from the visible file.
	for _, header := range current.Headers {
		if !visible[fmt.Sprintf("%t/%s", header.TLS, header.Scope)] {
			site.Headers = append(site.Headers, header)
		}
	}
	domain.NormalizeSite(&site)
	if err = domain.ValidateSiteSettings(site); err != nil {
		return current, err
	}
	canonical, err := WebsiteConfig(site, httpsPort)
	if err != nil {
		return current, err
	}
	expected, err := parseWebsiteConfig(canonical)
	if err != nil {
		return current, fmt.Errorf("生成网站配置失败: %w", err)
	}
	if err = configMismatch(parsed, expected, "website"); err != nil {
		return current, err
	}
	return site, nil
}

func importWebsiteServer(site *domain.Site, server components.IBlock, visible map[string]bool) error {
	ssl := false
	for _, d := range configDirectives(server) {
		if d.GetName() == "listen" && slices.Contains(configParams(d), "ssl") {
			ssl = true
		}
	}
	if names := configValues(server, "server_name"); len(names) > 0 && !slices.Equal(names, site.Domains) {
		var bindings []domain.DomainBinding
		for _, old := range site.Bindings {
			for _, name := range names {
				binding := domain.DomainBinding{Domain: name, Port: old.Port, SSL: old.SSL}
				if !slices.Contains(bindings, binding) {
					bindings = append(bindings, binding)
				}
			}
		}
		site.Bindings = bindings
		site.Domains = names
	}
	if root := configValues(server, "root"); len(root) == 1 {
		prefix := Mount + "/sites/" + site.ID
		if root[0] != prefix && !strings.HasPrefix(root[0], prefix+"/") {
			return fmt.Errorf("网站根目录必须位于 %s；只允许修改网络目录", prefix)
		}
		site.RunDirectory = strings.TrimPrefix(root[0], prefix)
		if site.RunDirectory == "" {
			site.RunDirectory = "/"
		}
		site.Index = configValues(server, "index")
		symlinks := configValues(server, "disable_symlinks")
		site.Security.FollowSymlinks = slices.Equal(symlinks, []string{"off"})
		site.Security.AllowHiddenFiles = true
		for _, d := range configDirectives(server) {
			if d.GetName() == "location" && slices.Equal(configParams(d), []string{"~", `/\.`}) {
				site.Security.AllowHiddenFiles = false
			}
		}
		importRealIP(site, server)
	}
	if ssl {
		if values := configValues(server, "ssl_protocols"); len(values) > 0 {
			site.HTTPS.Protocols = values
		}
		site.HTTPS.Ciphers = ""
		if values := configValues(server, "ssl_ciphers"); len(values) == 1 {
			site.HTTPS.Ciphers = values[0]
		}
		site.HTTPS.HTTP2 = slices.Equal(configValues(server, "http2"), []string{"on"})
		site.HTTPS.HSTS, site.HTTPS.HSTSIncludeSubDomains = false, false
	}
	importManagedHeaders(site, server, ssl)
	if err := importResponseHeaders(site, server, "server", ssl, visible); err != nil {
		return err
	}
	for _, d := range configDirectives(server) {
		if d.GetName() != "location" || d.GetBlock() == nil {
			continue
		}
		scope := websiteLocationScope(*site, configParams(d))
		if scope == "" {
			continue
		}
		if err := importResponseHeaders(site, d.GetBlock(), scope, ssl, visible); err != nil {
			return err
		}
		for i := range site.Proxies {
			if scope == "proxy:"+site.Proxies[i].ID {
				if err := importProxyConfig(&site.Proxies[i], d.GetBlock()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func websiteLocationScope(site domain.Site, params []string) string {
	for _, proxy := range site.Proxies {
		expected := []string{proxy.Match}
		if proxy.Modifier != "" {
			expected = append([]string{proxy.Modifier}, expected...)
		}
		if proxy.Enabled && slices.Equal(params, expected) {
			return "proxy:" + proxy.ID
		}
	}
	if slices.Equal(params, []string{"/"}) {
		return "root"
	}
	if len(params) == 2 && params[0] == "=" && strings.HasPrefix(params[1], "/__resty_errors/") && strings.HasSuffix(params[1], ".html") {
		return "error:" + strings.TrimSuffix(strings.TrimPrefix(params[1], "/__resty_errors/"), ".html")
	}
	return ""
}

func importResponseHeaders(site *domain.Site, block components.IBlock, scope string, ssl bool, visible map[string]bool) error {
	visible[fmt.Sprintf("%t/%s", ssl, scope)] = true
	for _, d := range configDirectives(block) {
		if d.GetName() != "add_header" {
			continue
		}
		params := configParams(d)
		if len(params) < 2 || len(params) > 3 || len(params) == 3 && params[2] != "always" || d.GetBlock() != nil {
			return fmt.Errorf("第 %d 行 add_header 需要名称、值及可选 always", d.GetLine())
		}
		if !domain.ManagedResponseHeader(params[0]) {
			site.Headers = append(site.Headers, domain.ResponseHeader{Scope: scope, TLS: ssl, Name: params[0], Value: params[1], Always: len(params) == 3})
		}
	}
	return nil
}
