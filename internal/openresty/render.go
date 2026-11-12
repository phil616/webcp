// Package openresty renders immutable deployment generations and controls the managed container.
package openresty

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/pki"
)

const Mount = "/srv/resty"

type Generation struct {
	ID              string
	Path            string
	Config          string
	Revision        uint64
	Ports           []PortBinding
	WebPorts        []WebsitePort
	ActiveHTTPPorts []int
}

// Render only produces files. Lua is written to separate files, never interpolated into nginx directives.
type RenderOptions struct {
	HTTPSPort   int
	WorkerUser  *string
	LoadModules []string
	CopySites   bool
	MIMEPath    string
}

func Render(root string, state domain.State, options ...RenderOptions) (gen Generation, err error) {
	state.Sites = slices.Clone(state.Sites)
	domain.NormalizeState(&state)
	if err = domain.Validate(state); err != nil {
		return gen, err
	}
	// Time-dependent checks belong to deployment, so an expired certificate never
	// prevents opening the manager to replace it or restoring an older backup.
	for _, site := range state.Sites {
		if !site.Enabled {
			continue
		}
		for _, c := range state.Certificates {
			if c.ID == site.TLSID || c.ID == site.ClientCAID {
				if time.Now().Before(c.NotBefore) || time.Now().After(c.NotAfter) {
					return gen, fmt.Errorf("网站 %s 的证书 %s 不在有效期内", site.Name, c.Name)
				}
			}
		}
	}
	gen.ID = domain.NewID()
	gen.Revision = state.Revision
	gen.Path = filepath.Join(root, "runtime", "generations", gen.ID)
	if err = os.MkdirAll(gen.Path, 0700); err != nil {
		return gen, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(gen.Path)
		}
	}()
	base := Mount + "/generations/" + gen.ID
	write := func(name, content string) error {
		path := filepath.Join(gen.Path, name)
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		return os.WriteFile(path, []byte(content), 0600)
	}
	for _, l := range state.Lua {
		folder := "blocks/" + l.ID
		if l.Kind == "module" {
			folder = "modules/" + l.Name
		}
		if err = write(folder+".lua", l.Code); err != nil {
			return gen, err
		}
	}
	usedCAs := map[string]bool{}
	for _, site := range state.Sites {
		if site.Enabled {
			if ca := domain.MTLSAuthority(state, site.ID); ca != "" {
				usedCAs[ca] = true
			}
		}
	}
	for _, c := range state.Certificates {
		if err = write("certs/"+c.ID+".pem", c.CertPEM); err != nil {
			return gen, err
		}
		if c.Kind == "server" {
			if err = write("certs/"+c.ID+".key", c.KeyPEM); err != nil {
				return gen, err
			}
		}
	}
	for _, ca := range state.MTLS.Authorities {
		if usedCAs[ca.ID] {
			if time.Now().Before(ca.NotBefore) || time.Now().After(ca.NotAfter) {
				return gen, fmt.Errorf("mTLS CA 不在有效期内")
			}
			crl, e := pki.CRL(ca, state.MTLS.Clients)
			if e != nil {
				return gen, e
			}
			if err = write("mtls/"+ca.ID+".pem", ca.CertPEM); err != nil {
				return gen, err
			}
			if err = write("mtls/"+ca.ID+".crl", string(crl)); err != nil {
				return gen, err
			}
		}
	}

	publicHTTPS := 443
	if len(options) > 0 && options[0].HTTPSPort > 0 {
		publicHTTPS = options[0].HTTPSPort
	}
	var b strings.Builder
	if len(options) > 0 {
		for _, module := range options[0].LoadModules {
			fmt.Fprintf(&b, "load_module %s;\n", quote(module))
		}
	}
	if len(options) > 0 && options[0].WorkerUser != nil {
		if *options[0].WorkerUser != "" {
			fmt.Fprintf(&b, "user %s;\n", *options[0].WorkerUser)
		}
	} else if os.Getuid() == 0 {
		b.WriteString("user root;\n")
	}
	b.WriteString("worker_processes auto;\npid /tmp/resty-manager.pid;\nerror_log /dev/stderr warn;\nevents { worker_connections 1024; }\nhttp {\n variables_hash_bucket_size 128;\n variables_hash_max_size 32768;\n map_hash_bucket_size 512;\n server_names_hash_max_size 65536;\n include /usr/local/openresty/nginx/conf/mime.types;\n default_type application/octet-stream;\n access_log /dev/stdout;\n server_tokens off;\n sendfile on;\n client_body_temp_path /tmp/client_body;\n proxy_temp_path /tmp/proxy;\n fastcgi_temp_path /tmp/fastcgi;\n uwsgi_temp_path /tmp/uwsgi;\n scgi_temp_path /tmp/scgi;\n map $http_upgrade $connection_upgrade { default upgrade; '' close; }\n map $request_method $resty_preflight_age { default ''; OPTIONS 1728000; }\n")
	performance := domain.EffectiveOpenResty(state.OpenResty)
	for _, name := range domain.OpenRestyDirectives(state.OpenResty) {
		fmt.Fprintf(&b, " %s %s;\n", name, performance.HTTP[name])
	}
	b.WriteString(" gzip_types text/plain text/css application/javascript application/json application/xml image/svg+xml;\n gzip_vary on;\n gzip_proxied any;\n")
	if len(options) > 0 && options[0].MIMEPath != "" {
		text := b.String()
		b.Reset()
		b.WriteString(strings.Replace(text, "/usr/local/openresty/nginx/conf/mime.types", options[0].MIMEPath, 1))
	}
	fmt.Fprintf(&b, " lua_package_path '%s/modules/?.lua;;';\n", base)
	var internal map[int]int
	internal, gen.WebPorts, err = websitePorts(state)
	if err != nil {
		return gen, err
	}
	listeners, _ := domain.WebsiteListeners(state)
	for port := range listeners {
		gen.ActiveHTTPPorts = append(gen.ActiveHTTPPorts, internal[port])
	}
	sitesRoot := Mount + "/sites"
	if len(options) > 0 && options[0].CopySites {
		sitesRoot = Mount + "/site-versions/" + gen.ID
	}
	renderHTTPGlobals(&b, state, internal, gen.WebPorts, publicHTTPS)
	for _, site := range state.Sites {
		if site.Enabled {
			site.ClientCAID = domain.MTLSAuthority(state, site.ID)
			if err = renderWebsite(&b, site, base, internal, publicHTTPS, sitesRoot); err != nil {
				return gen, err
			}
		}
	}
	b.WriteString("}\n")
	gen.Ports = renderStreams(&b, state.Streams)
	gen.Config = b.String()
	if err = write("nginx.conf", gen.Config); err != nil {
		return gen, err
	}
	if err = write("revision", fmt.Sprint(state.Revision)); err != nil {
		return gen, err
	}
	return gen, nil
}
