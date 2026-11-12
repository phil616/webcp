package httpapi

import (
	"net/http"
	"web-panel/internal/openresty"
	"web-panel/internal/service"
)

func registerRuntimeConnection(register func(string, endpoint), manager *service.Manager) {
	register("GET /api/runtime/connection", func(w http.ResponseWriter, r *http.Request) error {
		value, err := manager.RuntimeConnection()
		if err == nil {
			writeJSON(w, 200, value)
		}
		return err
	})
	register("GET /api/runtime/containers", func(w http.ResponseWriter, r *http.Request) error {
		value, err := manager.RuntimeContainers(r.Context())
		if err == nil {
			writeJSON(w, 200, value)
		}
		return err
	})
	register("POST /api/runtime/connection/probe", func(w http.ResponseWriter, r *http.Request) error {
		var connection openresty.Connection
		_, err := decode(w, r, &connection)
		if err != nil {
			return err
		}
		value, err := manager.ProbeRuntime(r.Context(), connection)
		if err == nil {
			writeJSON(w, 200, value)
		}
		return err
	})
	register("PUT /api/runtime/connection", func(w http.ResponseWriter, r *http.Request) error {
		var connection openresty.Connection
		version, err := decode(w, r, &connection)
		if err != nil {
			return err
		}
		return success(w, manager.ConfigureRuntime(r.Context(), version, connection))
	})
}
