package service

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"web-panel/internal/domain"
)

type File struct {
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

func (m *Manager) siteRoot(id string) (*os.Root, error) {
	s, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(s.Sites, func(v domain.Site) bool { return v.ID == id }) {
		return nil, ErrNotFound
	}
	return os.OpenRoot(filepath.Join(m.store.Root, "sites", id))
}
func safeFile(name string) error {
	if name == "" || path.Clean(name) != name || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("无效文件路径")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("不允许隐藏文件")
		}
	}
	lower := strings.ToLower(name)
	for _, suffix := range []string{".php", ".php3", ".php4", ".php5", ".php7", ".php8", ".phtml", ".phar"} {
		if strings.Contains(lower, suffix) {
			return fmt.Errorf("不支持 PHP 文件")
		}
	}
	return nil
}
func (m *Manager) Files(id string) ([]File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, err := m.siteRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := []File{}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("网站目录包含符号链接")
		}
		if entry.IsDir() {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		files = append(files, File{Path: name, Size: info.Size(), ModifiedAt: info.ModTime()})
		if len(files) > 10000 {
			return fmt.Errorf("文件数超过 10000")
		}
		return nil
	})
	return files, err
}
func (m *Manager) ReadFile(id, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := safeFile(name); err != nil {
		return nil, err
	}
	root, err := m.siteRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return nil, fmt.Errorf("仅能编辑 4 MiB 以内的普通文件")
	}
	data, err := io.ReadAll(io.LimitReader(f, 4<<20))
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("这是二进制文件，请通过上传功能替换")
	}
	return data, nil
}
func (m *Manager) WriteFile(revision uint64, id, name string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := safeFile(name); err != nil {
		return err
	}
	if len(content) > 4<<20 {
		return fmt.Errorf("文件超过 4 MiB")
	}
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	if revision != s.Revision {
		return ErrConflict
	}
	root, err := m.siteRoot(id)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	temp := path.Join(path.Dir(name), ".upload-"+domain.NewID())
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, err = f.Write(content)
	if err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = root.Rename(temp, name); err != nil {
		return err
	}
	if err = m.syncRuntimeFile(context.Background(), "sites", id, name, false); err != nil {
		return fmt.Errorf("文件已保存，但容器同步失败，请重新应用配置: %w", err)
	}
	return nil
}
func (m *Manager) DeleteFile(revision uint64, id, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := safeFile(name); err != nil {
		return err
	}
	s, err := m.store.Load()
	if err != nil {
		return err
	}
	if revision != s.Revision {
		return ErrConflict
	}
	root, err := m.siteRoot(id)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("只能删除普通文件")
	}
	if err = root.Remove(name); err != nil {
		return err
	}
	if err = m.syncRuntimeFile(context.Background(), "sites", id, name, true); err != nil {
		return fmt.Errorf("文件已删除，但容器同步失败，请重新应用配置: %w", err)
	}
	return nil
}
