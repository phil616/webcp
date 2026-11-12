package httpapi

import (
	"net/http"
	"web-panel/internal/domain"
	"web-panel/internal/service"
)

// Section-specific DTOs prevent a domain operation from replacing HTTPS or proxy settings.
func registerWebsiteSettings(register func(string, endpoint), m *service.Manager) {
	register("GET /api/sites/{id}/config", func(w http.ResponseWriter, r *http.Request) error {
		document, err := m.WebsiteConfig(r.PathValue("id"))
		if err == nil {
			writeJSON(w, 200, document)
		}
		return err
	})
	register("PUT /api/sites/{id}/config", func(w http.ResponseWriter, r *http.Request) error {
		var value struct {
			Content string `json:"content"`
		}
		revision, err := decode(w, r, &value)
		if err != nil {
			return err
		}
		return success(w, m.SaveWebsiteConfig(revision, r.PathValue("id"), value.Content))
	})
	register("PUT /api/sites/{id}/advanced", func(w http.ResponseWriter, r *http.Request) error {
		var value service.WebsiteAdvancedUpdate
		revision, err := decode(w, r, &value)
		if err != nil {
			return err
		}
		return success(w, m.SaveWebsiteAdvanced(revision, r.PathValue("id"), value))
	})
	register("PUT /api/sites/{id}/name", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Name string `json:"name"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveWebsiteName(rev, r.PathValue("id"), v.Name))
	})
	register("PUT /api/sites/{id}/extensions", func(w http.ResponseWriter, r *http.Request) error {
		var v service.ExtensionUpdate
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveExtensions(rev, r.PathValue("id"), v))
	})
	register("PUT /api/sites/{id}/proxy-cache", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.ProxyCacheConfig
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveProxyCache(rev, r.PathValue("id"), v))
	})
	for _, kind := range []string{"proxies", "redirects"} {
		register("GET /api/sites/{id}/"+kind+"/{rule}/config", func(w http.ResponseWriter, r *http.Request) error {
			config, e := m.RulePreview(r.PathValue("id"), r.PathValue("rule"), kind)
			if e == nil {
				writeJSON(w, 200, map[string]string{"config": config})
			}
			return e
		})
	}
	register("GET /api/sites/{id}/directories", func(w http.ResponseWriter, r *http.Request) error {
		v, e := m.Directories(r.PathValue("id"))
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("PUT /api/sites/{id}/domains", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Bindings []domain.DomainBinding `json:"bindings"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveBindings(rev, r.PathValue("id"), v.Bindings))
	})
	register("PUT /api/sites/{id}/directory", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			RunDirectory string `json:"runDirectory"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveDirectory(rev, r.PathValue("id"), v.RunDirectory))
	})
	register("PUT /api/sites/{id}/index", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Index []string `json:"index"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveIndex(rev, r.PathValue("id"), v.Index))
	})
	register("PUT /api/sites/{id}/https", func(w http.ResponseWriter, r *http.Request) error {
		var v service.HTTPSUpdate
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveHTTPS(rev, r.PathValue("id"), v))
	})
	register("PUT /api/sites/{id}/cors", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.CORSConfig
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveCORS(rev, r.PathValue("id"), v))
	})
	register("PUT /api/sites/{id}/real-ip", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.RealIPConfig
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, m.SaveRealIP(rev, r.PathValue("id"), v))
	})
	register("POST /api/sites/{id}/proxies", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.ProxyRule
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		v, e = m.SaveProxy(rev, r.PathValue("id"), v)
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("DELETE /api/sites/{id}/proxies/{rule}", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, m.DeleteProxy(rev, r.PathValue("id"), r.PathValue("rule")))
	})
	register("POST /api/sites/{id}/proxy-cache/clear", func(w http.ResponseWriter, r *http.Request) error {
		return success(w, m.ClearProxyCache(r.Context(), r.PathValue("id")))
	})
	register("POST /api/sites/{id}/redirects", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.RedirectRule
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		v, e = m.SaveRedirect(rev, r.PathValue("id"), v)
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("DELETE /api/sites/{id}/redirects/{rule}", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, m.DeleteRedirect(rev, r.PathValue("id"), r.PathValue("rule")))
	})
}
