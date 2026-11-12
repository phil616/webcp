// Package errorpages edits OpenResty document-root files. It never stores HTML in a database.
package errorpages

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
	"web-panel/internal/domain"
	"web-panel/internal/fsutil"
)

//go:embed templates/*.html
var defaults embed.FS
var Codes = []string{"403", "404", "495", "496", "500", "502", "503", "504"}
var ErrConflict = errors.New("页面文件已变化，请重新加载后编辑")

type Document struct {
	Pages map[string]string `json:"pages"`
	ETag  string            `json:"etag"`
}
type Files struct{ Root string }

func New(dataRoot string) Files { return Files{Root: filepath.Join(dataRoot, "runtime", "root")} }
func (f Files) directory(site string) (string, error) {
	if site == "" {
		return filepath.Join(f.Root, "default"), nil
	}
	if !domain.ValidID(site) {
		return "", fmt.Errorf("无效网站 ID")
	}
	return filepath.Join(f.Root, "sites", site), nil
}
func (f Files) Read(site string) (Document, error) {
	dir, err := f.directory(site)
	if err != nil {
		return Document{}, err
	}
	result := Document{Pages: map[string]string{}}
	for _, folder := range []string{f.Root, filepath.Join(f.Root, "sites"), dir} {
		info, e := os.Lstat(folder)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return result, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("页面目录不可为符号链接或特殊文件")
		}
	}
	for _, code := range Codes {
		file := filepath.Join(dir, code+".html")
		info, e := os.Lstat(file)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return result, e
		}
		if !info.Mode().IsRegular() || info.Size() > 256<<10 {
			return result, fmt.Errorf("错误页面必须为不超过 256 KiB 的普通文件: %s", code)
		}
		input, e := os.Open(file)
		if e != nil {
			return result, e
		}
		content, e := io.ReadAll(io.LimitReader(input, (256<<10)+1))
		input.Close()
		if e != nil {
			return result, e
		}
		result.Pages[code] = string(content)
	}
	// Empty files remain readable so the editor can repair an external edit.
	// Replace and backup import enforce the nonempty content requirement.
	data, _ := json.Marshal(result.Pages)
	digest := sha256.Sum256(data)
	result.ETag = hex.EncodeToString(digest[:])
	return result, nil
}

// Replace uses an atomic directory exchange: readers see either the complete old or new scope.
// Files have their own content ETag, independent of the controller's configuration revision.
func (f Files) Replace(site, expected string, pages map[string]string) error {
	if pages == nil {
		return fmt.Errorf("页面必须为 JSON 对象")
	}
	if err := domain.ValidatePages(pages); err != nil {
		return err
	}
	dir, err := f.directory(site)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	current, err := f.Read(site)
	if err != nil {
		return err
	}
	if expected != "" && current.ETag != expected {
		return ErrConflict
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !ValidateRelative("default/"+entry.Name()) {
			return fmt.Errorf("页面目录包含非受管资源 %s，拒绝覆盖；请将额外资源移到独立目录", entry.Name())
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".pages-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for code, content := range pages {
		if err = fsutil.AtomicWrite(filepath.Join(stage, code+".html"), []byte(content), 0600); err != nil {
			return err
		}
	}
	// Recheck after staging to catch edits made by another filesystem client.
	latest, err := f.Read(site)
	if err != nil {
		return err
	}
	if latest.ETag != current.ETag {
		return ErrConflict
	}
	if err = unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, dir, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("原子替换 OpenResty 页面目录失败: %w", err)
	}
	parent, err := os.Open(filepath.Dir(dir))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

// Ensure initializes missing engine assets once and imports legacy HTML without overwriting file edits.
func (f Files) Ensure(legacy map[string]map[string]string) error {
	for scope, pages := range legacy {
		dir, err := f.directory(scope)
		if err != nil {
			return err
		}
		if err = domain.ValidatePages(pages); err != nil {
			return err
		}
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		for code, html := range pages {
			file := filepath.Join(dir, code+".html")
			if _, err = os.Lstat(file); os.IsNotExist(err) {
				if err = fsutil.AtomicWrite(file, []byte(html), 0600); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(filepath.Join(f.Root, ".initialized")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	dir, _ := f.directory("")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, code := range Codes {
		file := filepath.Join(dir, code+".html")
		if _, err := os.Lstat(file); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		source := "50x"
		if code == "404" {
			source = "404"
		}
		html, err := defaults.ReadFile("templates/" + source + ".html")
		if err != nil {
			return err
		}
		html = []byte(strings.ReplaceAll(string(html), "{{STATUS}}", code))
		if err = fsutil.AtomicWrite(file, html, 0600); err != nil {
			return err
		}
	}
	if _, err := f.Read(""); err != nil {
		return err
	}
	return f.MarkInitialized()
}

// ValidateRelative restricts backup extraction to engine-owned HTML resources.
func ValidateRelative(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) == 2 && parts[0] == "default" {
		return slices.Contains(Codes, strings.TrimSuffix(parts[1], ".html")) && strings.HasSuffix(parts[1], ".html")
	}
	if len(parts) == 3 && parts[0] == "sites" && domain.ValidID(parts[1]) {
		return slices.Contains(Codes, strings.TrimSuffix(parts[2], ".html")) && strings.HasSuffix(parts[2], ".html")
	}
	return false
}

// MarkInitialized records bootstrap completion without regenerating missing files.
// Restoring a current archive must preserve deliberately removed page assets.
func (f Files) MarkInitialized() error {
	for _, dir := range []string{filepath.Join(f.Root, "default"), filepath.Join(f.Root, "sites")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return fsutil.AtomicWrite(filepath.Join(f.Root, ".initialized"), []byte("OpenResty page assets v1\n"), 0600)
}
