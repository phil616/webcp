package httpapi

import (
	"fmt"
	"net/http"
	"web-panel/internal/service"
)

func registerOpenResty(register func(string, endpoint), m *service.Manager) {
	register("POST /api/panel/openresty/scope", func(w http.ResponseWriter, r *http.Request) error {
		var req struct {
			Scope string `json:"scope"`
		}
		_, err := decode(w, r, &req)
		if err != nil {
			return err
		}
		if req.Scope != "http-per" {
			return fmt.Errorf("不支持的 OpenResty 配置范围")
		}
		params, err := m.OpenRestyPerformance()
		if err == nil {
			writeJSON(w, 200, params)
		}
		return err
	})
	register("POST /api/panel/openresty/update", func(w http.ResponseWriter, r *http.Request) error {
		var req struct {
			Scope   string            `json:"scope"`
			Operate string            `json:"operate"`
			Params  map[string]string `json:"params"`
		}
		rev, err := decode(w, r, &req)
		if err != nil {
			return err
		}
		if req.Scope != "http-per" || req.Operate != "update" {
			return fmt.Errorf("不支持的 OpenResty 操作")
		}
		return success(w, m.SaveOpenRestyPerformance(r.Context(), rev, req.Params))
	})
	// No dynamic binary modules are managed by this standalone program.
	register("GET /api/panel/openresty/modules", func(w http.ResponseWriter, r *http.Request) error {
		writeJSON(w, 200, map[string]any{"modules": []any{}})
		return nil
	})
}
