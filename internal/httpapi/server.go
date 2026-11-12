// Package httpapi translates HTTP requests into application commands. It has no persistence logic.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"web-panel/internal/buildinfo"
	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
	"web-panel/internal/service"
)

type endpoint func(http.ResponseWriter, *http.Request) error

func newAPI(manager *service.Manager, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	register := func(pattern string, fn endpoint) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if err := fn(w, r); err != nil {
				status := http.StatusBadRequest
				switch {
				case errors.Is(err, service.ErrNotFound), errors.Is(err, os.ErrNotExist):
					status = 404
				case errors.Is(err, service.ErrConflict):
					status = 409
				}
				writeJSON(w, status, map[string]string{"error": err.Error()})
			}
		})
	}
	register("GET /api/version", func(w http.ResponseWriter, r *http.Request) error {
		writeJSON(w, http.StatusOK, buildinfo.Current())
		return nil
	})
	registerRuntimeConnection(register, manager)
	registerOpenResty(register, manager)
	registerWebsiteSettings(register, manager)
	registerCertificates(register, manager)
	register("GET /api/overview", func(w http.ResponseWriter, r *http.Request) error {
		v, e := manager.Overview(r.Context())
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("POST /api/sites", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.Site
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		v, e = manager.SaveSite(rev, v)
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("DELETE /api/sites/{id}", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, manager.DeleteSite(rev, r.PathValue("id")))
	})
	register("POST /api/lua", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.Lua
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		v, e = manager.SaveLua(rev, v)
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("POST /api/streams", func(w http.ResponseWriter, r *http.Request) error {
		var proxy domain.StreamProxy
		rev, err := decode(w, r, &proxy)
		if err != nil {
			return err
		}
		proxy, err = manager.SaveStream(rev, proxy)
		if err == nil {
			writeJSON(w, 200, proxy)
		}
		return err
	})
	register("DELETE /api/streams/{id}", func(w http.ResponseWriter, r *http.Request) error {
		rev, err := revision(r)
		if err != nil {
			return err
		}
		return success(w, manager.DeleteStream(rev, r.PathValue("id")))
	})
	register("DELETE /api/lua/{id}", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, manager.DeleteLua(rev, r.PathValue("id")))
	})
	register("GET /api/pages", func(w http.ResponseWriter, r *http.Request) error {
		value, err := manager.ReadPages("")
		if err == nil {
			writeJSON(w, 200, value)
		}
		return err
	})
	register("GET /api/sites/{id}/pages", func(w http.ResponseWriter, r *http.Request) error {
		value, err := manager.ReadPages(r.PathValue("id"))
		if err == nil {
			writeJSON(w, 200, value)
		}
		return err
	})
	for _, pattern := range []string{"PUT /api/pages", "PUT /api/sites/{id}/pages"} {
		register(pattern, func(w http.ResponseWriter, r *http.Request) error {
			var value errorpages.Document
			rev, err := decode(w, r, &value)
			if err != nil {
				return err
			}
			return success(w, manager.UpdatePages(rev, r.PathValue("id"), value))
		})
	}

	register("PUT /api/s3", func(w http.ResponseWriter, r *http.Request) error {
		var v domain.S3
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, manager.SaveS3(rev, v))
	})
	register("POST /api/s3/check", func(w http.ResponseWriter, r *http.Request) error { return success(w, manager.CheckS3(r.Context())) })

	register("GET /api/config", func(w http.ResponseWriter, r *http.Request) error {
		text, e := manager.Preview()
		if e == nil {
			writeJSON(w, 200, map[string]string{"config": text})
		}
		return e
	})
	register("POST /api/runtime/apply", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, manager.Apply(r.Context(), rev))
	})
	register("POST /api/runtime/stop", func(w http.ResponseWriter, r *http.Request) error { return success(w, manager.Stop(r.Context())) })
	register("GET /api/runtime/logs", func(w http.ResponseWriter, r *http.Request) error {
		text, e := manager.Logs(r.Context())
		if e == nil {
			writeJSON(w, 200, map[string]string{"logs": text})
		}
		return e
	})
	register("GET /api/sites/{id}/files", func(w http.ResponseWriter, r *http.Request) error {
		v, e := manager.Files(r.PathValue("id"))
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("GET /api/sites/{id}/file", func(w http.ResponseWriter, r *http.Request) error {
		v, e := manager.ReadFile(r.PathValue("id"), r.URL.Query().Get("path"))
		if e == nil {
			writeJSON(w, 200, map[string]string{"content": string(v)})
		}
		return e
	})
	register("PUT /api/sites/{id}/file", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, manager.WriteFile(rev, r.PathValue("id"), v.Path, []byte(v.Content)))
	})
	register("DELETE /api/sites/{id}/file", func(w http.ResponseWriter, r *http.Request) error {
		rev, e := revision(r)
		if e != nil {
			return e
		}
		return success(w, manager.DeleteFile(rev, r.PathValue("id"), r.URL.Query().Get("path")))
	})
	register("POST /api/sites/{id}/upload", func(w http.ResponseWriter, r *http.Request) error {
		rev, err := revision(r)
		if err != nil {
			return err
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			return fmt.Errorf("需要 application/octet-stream 上传")
		}
		content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			return err
		}
		return success(w, manager.WriteFile(rev, r.PathValue("id"), r.URL.Query().Get("path"), content))
	})
	register("GET /api/backups", func(w http.ResponseWriter, r *http.Request) error {
		v, e := manager.Backups(r.Context(), r.URL.Query().Get("remote") == "true")
		if e == nil {
			writeJSON(w, 200, v)
		}
		return e
	})
	register("POST /api/backups", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Remote bool `json:"remote"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		record, e := manager.Backup(r.Context(), rev, v.Remote)
		if e == nil {
			writeJSON(w, 200, record)
		}
		return e
	})
	register("POST /api/backups/{id}/restore", func(w http.ResponseWriter, r *http.Request) error {
		var v struct {
			Remote bool `json:"remote"`
		}
		rev, e := decode(w, r, &v)
		if e != nil {
			return e
		}
		return success(w, manager.Restore(r.Context(), rev, r.PathValue("id"), v.Remote))
	})
	register("GET /api/backups/{id}/download", func(w http.ResponseWriter, r *http.Request) error {
		f, e := manager.OpenBackup(r.PathValue("id"))
		if e != nil {
			return e
		}
		defer f.Close()
		stat, e := f.Stat()
		if e != nil {
			return e
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="webcp-backup.tar.gz"`)
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
		return nil
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, map[string]string{"error": "API 不存在"})
	})
	mux.Handle("/", http.FileServerFS(assets))
	protection := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				slog.Error("request panicked", "path", r.URL.Path, "panic", value)
				writeJSON(w, 500, map[string]string{"error": "内部错误，请查看服务日志"})
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		protection.ServeHTTP(w, r)
	})
}

func revision(r *http.Request) (uint64, error) {
	v, e := strconv.ParseUint(strings.Trim(r.Header.Get("If-Match"), `"`), 10, 64)
	if e != nil || v == 0 {
		return 0, fmt.Errorf("需要 If-Match 配置版本")
	}
	return v, nil
}
func decode(w http.ResponseWriter, r *http.Request, v any) (uint64, error) {
	rev, err := revision(r)
	if err != nil {
		return 0, err
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return 0, fmt.Errorf("需要 application/json 请求")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(v); err != nil {
		return 0, fmt.Errorf("无效请求: %w", err)
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return 0, fmt.Errorf("请求必须是单个 JSON 对象")
	}
	return rev, nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write response", "error", err)
	}
}
func success(w http.ResponseWriter, err error) error {
	if err == nil {
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
	return err
}

func Server(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
}
