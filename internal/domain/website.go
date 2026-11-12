package domain

import (
	"slices"
)

// Website configuration is typed and persisted; raw nginx fragments are never stored.
type ResponseHeader struct {
	Scope  string `json:"scope"` // server, root, proxy:<id>, error:<status>
	TLS    bool   `json:"tls"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Always bool   `json:"always"`
}

type WebsiteSecurity struct {
	FollowSymlinks   bool `json:"followSymlinks"`
	AllowHiddenFiles bool `json:"allowHiddenFiles"`
}

type ProxyCacheConfig struct {
	MemoryMB        int `json:"memoryMb"`
	MaxSizeMB       int `json:"maxSizeMb"`
	InactiveMinutes int `json:"inactiveMinutes"`
}

type DomainBinding struct {
	Domain string `json:"domain"`
	Port   int    `json:"port"`
	SSL    bool   `json:"ssl"`
}
type HTTPSConfig struct {
	HTTPConfig            string   `json:"httpConfig"`
	Protocols             []string `json:"protocols"`
	Ciphers               string   `json:"ciphers"`
	HSTS                  bool     `json:"hsts"`
	HSTSIncludeSubDomains bool     `json:"hstsIncludeSubDomains"`
	HTTP2                 bool     `json:"http2"`
	HTTP3                 bool     `json:"http3"`
}
type CORSConfig struct {
	Enabled          bool   `json:"enabled"`
	AllowOrigins     string `json:"allowOrigins"`
	AllowMethods     string `json:"allowMethods"`
	AllowHeaders     string `json:"allowHeaders"`
	AllowCredentials bool   `json:"allowCredentials"`
	Preflight        bool   `json:"preflight"`
}
type RealIPConfig struct {
	Enabled bool     `json:"enabled"`
	From    []string `json:"from"`
	Header  string   `json:"header"`
}
type Replacement struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type ProxyRule struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Enabled         bool          `json:"enabled"`
	Modifier        string        `json:"modifier"`
	Match           string        `json:"match"`
	ProxyPass       string        `json:"proxyPass"`
	ProxyHost       string        `json:"proxyHost"`
	SNI             bool          `json:"sni"`
	SSLName         string        `json:"sslName"`
	SSLVerify       bool          `json:"sslVerify"`
	ConnectTimeout  int           `json:"connectTimeout"`
	ReadTimeout     int           `json:"readTimeout"`
	SendTimeout     int           `json:"sendTimeout"`
	Cache           bool          `json:"cache"`
	ServerCacheTime int           `json:"serverCacheTime"`
	ServerCacheUnit string        `json:"serverCacheUnit"`
	CacheTime       int           `json:"cacheTime"`
	CacheUnit       string        `json:"cacheUnit"`
	Replaces        []Replacement `json:"replaces"`
	CORS            CORSConfig    `json:"cors"`
}
type RedirectRule struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Type         string   `json:"type"`
	Domains      []string `json:"domains"`
	Path         string   `json:"path"`
	Redirect     int      `json:"redirect"`
	Target       string   `json:"target"`
	KeepPath     bool     `json:"keepPath"`
	RedirectRoot bool     `json:"redirectRoot"`
}

func DefaultHTTPS() HTTPSConfig {
	return HTTPSConfig{HTTPConfig: "HTTPToHTTPS", Protocols: []string{"TLSv1.3", "TLSv1.2"}, HTTP2: true}
}
func DefaultCORS() CORSConfig {
	return CORSConfig{AllowOrigins: "*", AllowMethods: "GET,POST,OPTIONS,PUT,DELETE", Preflight: true}
}
func NormalizeSite(s *Site) {
	if s.Headers == nil {
		s.Headers = []ResponseHeader{}
	}
	if s.LuaBindings == nil {
		s.LuaBindings = map[string]string{}
	}
	if s.RealIP.From == nil {
		s.RealIP.From = []string{}
	}
	if s.ProxyCache == nil {
		s.ProxyCache = &ProxyCacheConfig{MemoryMB: 5, MaxSizeMB: 1024, InactiveMinutes: 60}
	}
	if s.Bindings == nil {
		s.Bindings = []DomainBinding{}
		for _, host := range s.Domains {
			s.Bindings = append(s.Bindings, DomainBinding{Domain: host, Port: 80})
		}
	}
	s.Domains = []string{}
	for _, binding := range s.Bindings {
		if !slices.Contains(s.Domains, binding.Domain) {
			s.Domains = append(s.Domains, binding.Domain)
		}
	}
	if s.RunDirectory == "" {
		s.RunDirectory = "/"
	}
	if s.Index == nil {
		s.Index = []string{"index.html"}
	}
	if s.HTTPS == nil {
		v := DefaultHTTPS()
		s.HTTPS = &v
	}
	if s.Proxies == nil {
		s.Proxies = EffectiveProxies(*s)
		if s.Proxies == nil {
			s.Proxies = []ProxyRule{}
		}
	}
	if s.Redirects == nil {
		s.Redirects = []RedirectRule{}
	}
}
func SiteDefaults(s Site) Site { NormalizeSite(&s); return s }
func EffectiveProxies(s Site) []ProxyRule {
	// nil is the legacy single-origin site; an explicit empty list has no active proxies.
	if s.Proxies == nil && s.Kind == "proxy" && s.Upstream != "" {
		return []ProxyRule{{ID: s.ID, Name: "default", Enabled: true, Match: "/", ProxyPass: s.Upstream, ProxyHost: "$host", SNI: true, SSLVerify: true, ConnectTimeout: 60, ReadTimeout: 60, SendTimeout: 60, ServerCacheTime: 10, ServerCacheUnit: "m", CacheUnit: "h", Replaces: []Replacement{}, CORS: DefaultCORS()}}
	}
	return s.Proxies
}
func NormalizeState(s *State) {
	if s.NextSiteID == 0 {
		s.NextSiteID = 1
	}
	maxID := s.NextSiteID - 1
	for _, site := range s.Sites {
		if site.PanelID > maxID {
			maxID = site.PanelID
		}
	}
	for i := range s.Sites {
		if s.Sites[i].PanelID == 0 {
			maxID++
			s.Sites[i].PanelID = maxID
		}
	}
	s.NextSiteID = maxID + 1
	normalizeCertificates(s)
	if s.Streams == nil {
		s.Streams = []StreamProxy{}
	}
	for i := range s.Sites {
		site := &s.Sites[i]
		if site.Proxies == nil {
			site.Proxies = EffectiveProxies(*site)
		}
		NormalizeSite(site)
	}
}
