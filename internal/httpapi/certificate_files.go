package httpapi

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"web-panel/internal/service"
)

func panelFiles(w http.ResponseWriter, r *http.Request, path string) error {
	var v struct {
		Path     string `json:"path"`
		Name     string `json:"name"`
		IsDir    bool   `json:"isDir"`
		IsDetail bool   `json:"isDetail"`
		Dir      bool   `json:"dir"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	if e := panelDecode(r, &v); e != nil {
		return e
	}
	if v.Path == "" {
		v.Path = "/"
	}
	if !filepath.IsAbs(v.Path) {
		return fmt.Errorf("需要绝对路径")
	}
	switch path {
	case "standalone/files/search":
		entries, e := os.ReadDir(v.Path)
		if e != nil {
			return e
		}
		items := []map[string]any{}
		for _, entry := range entries {
			info, e := entry.Info()
			if e != nil {
				continue
			}
			if v.Dir && !entry.IsDir() {
				continue
			}
			items = append(items, map[string]any{"name": entry.Name(), "path": filepath.Join(v.Path, entry.Name()), "isDir": entry.IsDir(), "size": info.Size(), "modTime": info.ModTime(), "extension": filepath.Ext(entry.Name())})
		}
		writeJSON(w, 200, map[string]any{"items": items, "itemTotal": len(items), "path": v.Path, "name": filepath.Base(v.Path)})
	case "standalone/files/create":
		if v.IsDir {
			if e := os.Mkdir(v.Path, 0700); e != nil {
				return e
			}
		} else {
			f, e := os.OpenFile(v.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return e
			}
			if e = f.Close(); e != nil {
				return e
			}
		}

		writeJSON(w, 200, map[string]bool{"success": true})
	case "standalone/files/size":
		var size int64
		err := filepath.WalkDir(v.Path, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if e = r.Context().Err(); e != nil {
				return e
			}
			if d.Type().IsRegular() {
				info, e := d.Info()
				if e != nil {
					return e
				}
				size += info.Size()
			}
			return nil
		})
		if err != nil {
			return err
		}
		writeJSON(w, 200, map[string]int64{"size": size})
	case "standalone/files/content":
		if v.IsDetail {
			info, e := os.Stat(v.Path)
			if e != nil {
				return e
			}
			writeJSON(w, 200, map[string]int64{"size": info.Size()})
			return nil
		}
		text, e := service.ReadPEMFile(v.Path)
		data := []byte(text)
		if e != nil {
			return e
		}
		if len(data) > 1<<20 {
			return fmt.Errorf("文件过大")
		}
		writeJSON(w, 200, map[string]string{"content": string(data)})
	default:
		return fmt.Errorf("不支持的证书文件操作")
	}
	return nil
}
