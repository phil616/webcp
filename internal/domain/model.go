// Package domain defines the persisted configuration, independent of HTTP and Docker.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

const SchemaVersion = 2
const MaxStateSize = 16 << 20

type State struct {
	OpenResty    *OpenRestyConfig `json:"openresty,omitempty"`
	NextSiteID   uint             `json:"nextSiteId"`
	SSL          SSLState         `json:"ssl"`
	MTLS         MTLSState        `json:"mtls"`
	Schema       int              `json:"schema"`
	Revision     uint64           `json:"revision"`
	Sites        []Site           `json:"sites"`
	Streams      []StreamProxy    `json:"streams"`
	Lua          []Lua            `json:"lua"`
	Certificates []Certificate    `json:"certificates"`
	S3           S3               `json:"s3"`
}

type Site struct {
	PanelID      uint              `json:"panelId"`
	ProxyCache   *ProxyCacheConfig `json:"proxyCache"`
	Bindings     []DomainBinding   `json:"bindings"`
	RunDirectory string            `json:"runDirectory"`
	Index        []string          `json:"index"`
	HTTPS        *HTTPSConfig      `json:"https"`
	CORS         CORSConfig        `json:"cors"`
	RealIP       RealIPConfig      `json:"realIp"`
	Proxies      []ProxyRule       `json:"proxies"`
	Redirects    []RedirectRule    `json:"redirects"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Domains      []string          `json:"domains"`
	Kind         string            `json:"kind"` // static or proxy (one origin)
	Upstream     string            `json:"upstream"`
	Enabled      bool              `json:"enabled"`
	TLSID        string            `json:"tlsId"`
	ClientCAID   string            `json:"clientCaId,omitempty"`
	LuaBindings  map[string]string `json:"luaBindings"` // phase -> reusable block ID
	CreatedAt    time.Time         `json:"createdAt"`
}

type Lua struct {
	ID          string `json:"id"`
	Name        string `json:"name"` // require(name) for modules
	Kind        string `json:"kind"` // module or block
	Description string `json:"description"`
	Code        string `json:"code"`
}

type Certificate struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Kind      string     `json:"kind"` // ca, server, client
	CAID      string     `json:"caId"`
	Serial    string     `json:"serial"`
	Domains   []string   `json:"domains"`
	CertPEM   string     `json:"certPem"`
	KeyPEM    string     `json:"keyPem,omitempty"` // never returned in list responses
	NotBefore time.Time  `json:"notBefore"`
	NotAfter  time.Time  `json:"notAfter"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

type S3 struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey,omitempty"`
	PathStyle bool   `json:"pathStyle"`
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func Default() State {
	return State{Schema: SchemaVersion, Revision: 1, Sites: []Site{}, Streams: []StreamProxy{}, Lua: []Lua{}, Certificates: []Certificate{}}
}
