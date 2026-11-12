// Package backup owns the versioned backup format and S3 transport, without UI dependencies.
package backup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/errorpages"
)

const MaxArchiveSize int64 = 2 << 30
const MaxFileSize int64 = 128 << 20

type Record struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	Revision  uint64    `json:"revision"`
	ObjectKey string    `json:"objectKey,omitempty"`
	Remote    bool      `json:"remote"`
}

func Create(root string, state domain.State) (record Record, err error) {
	for _, scope := range append([]string{""}, siteIDs(state.Sites)...) {
		document, e := errorpages.New(root).Read(scope)
		if e != nil {
			return record, e
		}
		if e = domain.ValidatePages(document.Pages); e != nil {
			return record, e
		}
	}
	record = Record{ID: domain.NewID(), CreatedAt: time.Now().UTC(), Revision: state.Revision}
	file, err := os.OpenFile(filepath.Join(root, "backups", record.ID+".tar.gz"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return record, err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(file, hash))
	tw := tar.NewWriter(gz)
	defer gz.Close()
	defer tw.Close()
	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return record, err
	}
	if len(stateJSON) > domain.MaxStateSize {
		return record, fmt.Errorf("配置快照超过 16 MiB 限制")
	}
	if err = tw.WriteHeader(&tar.Header{Name: "state.json", Mode: 0600, Size: int64(len(stateJSON)), ModTime: record.CreatedAt}); err != nil {
		return record, err
	}
	if _, err = tw.Write(stateJSON); err != nil {
		return record, err
	}
	total := int64(len(stateJSON))
	count := 1 // state.json participates in the same restore limits as website entries.
	type source struct{ host, archive string }
	sources := []source{{filepath.Join(root, "runtime/root/default"), "openresty/root/default"}}
	for _, site := range state.Sites {
		sources = append(sources, source{filepath.Join(root, "sites", site.ID), "sites/" + site.ID}, source{filepath.Join(root, "runtime/root/sites", site.ID), "openresty/root/sites/" + site.ID})
	}
	for _, item := range sources {
		siteRoot := item.host
		err = filepath.WalkDir(siteRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) && filePath == siteRoot {
					return nil
				}
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("备份不接受符号链接: %s", filePath)
			}
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("不支持特殊文件: %s", filePath)
			}
			count++
			total += info.Size()
			if count > 100000 || total > MaxArchiveSize || info.Size() > MaxFileSize {
				return fmt.Errorf("备份超出大小限制")
			}
			rel, e := filepath.Rel(siteRoot, filePath)
			if e != nil {
				return e
			}
			header, e := tar.FileInfoHeader(info, "")
			if e != nil {
				return e
			}
			header.Name = item.archive
			if rel != "." {
				header.Name += "/" + filepath.ToSlash(rel)
			}
			if strings.HasPrefix(header.Name, "openresty/root/") && !info.IsDir() && !errorpages.ValidateRelative(strings.TrimPrefix(header.Name, "openresty/root/")) {
				return fmt.Errorf("默认页面目录包含非受管资源: %s", header.Name)
			}
			header.Uid = 0
			header.Gid = 0
			header.Uname = ""
			header.Gname = ""
			if e = tw.WriteHeader(header); e != nil {
				return e
			}
			if info.IsDir() {
				return nil
			}
			f, e := os.Open(filePath)
			if e != nil {
				return e
			}
			defer f.Close()
			n, e := io.CopyN(tw, f, info.Size())
			if e != nil {
				return e
			}
			if n != info.Size() {
				return io.ErrUnexpectedEOF
			}
			return nil
		})
		if err != nil {
			return record, err
		}
	}
	if err = tw.Close(); err != nil {
		return record, err
	}
	if err = gz.Close(); err != nil {
		return record, err
	}
	if err = file.Sync(); err != nil {
		return record, err
	}
	info, err := file.Stat()
	if err != nil {
		return record, err
	}
	record.Size = info.Size()
	if record.Size > MaxArchiveSize {
		return record, fmt.Errorf("压缩备份超过 2 GiB 限制")
	}
	record.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return record, nil
}

// Extract validates all archive paths, types, limits and state before a caller commits anything.
// The caller must supply a fresh, private staging directory.
func Extract(reader io.Reader, stage string) (state domain.State, err error) {
	compressed := bufio.NewReader(io.LimitReader(reader, MaxArchiveSize+1))
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return state, err
	}
	defer gz.Close()
	gz.Multistream(false)
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var total int64
	count := 0
	foundState := false
	legacyExport := false
	legacyPages := map[string]map[string]string{}
	if err = os.MkdirAll(filepath.Join(stage, "sites"), 0700); err != nil {
		return state, err
	}
	for {
		header, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return state, e
		}
		name := header.Name
		count++
		total += header.Size
		if count > 100000 || total > MaxArchiveSize || header.Size < 0 || header.Size > MaxFileSize {
			return state, fmt.Errorf("备份超出大小限制")
		}
		if path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			return state, fmt.Errorf("不安全或重复的备份路径: %s", name)
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return state, fmt.Errorf("备份中不允许链接或特殊文件")
		}
		if name == "state.json" {
			if header.Typeflag != tar.TypeReg || header.Size > domain.MaxStateSize {
				return state, fmt.Errorf("无效 state.json")
			}
			b, e := io.ReadAll(tr)
			if e != nil {
				return state, e
			}
			var version struct {
				Schema int `json:"schema"`
			}
			if e = json.Unmarshal(b, &version); e != nil {
				return state, e
			}
			legacyExport = version.Schema == 1
			if state, legacyPages, e = domain.DecodeExport(b); e != nil {
				return state, e
			}
			foundState = true
			continue
		}
		parts := strings.Split(name, "/")
		target := filepath.Join(stage, filepath.FromSlash(name))
		if strings.HasPrefix(name, "openresty/root/") {
			relative := strings.TrimPrefix(name, "openresty/root/")
			validDir := relative == "default" || relative == "sites" || strings.HasPrefix(relative, "sites/") && domain.ValidID(strings.TrimPrefix(relative, "sites/"))
			if header.Typeflag == tar.TypeDir && !validDir || header.Typeflag == tar.TypeReg && (!errorpages.ValidateRelative(relative) || header.Size > 256<<10) {
				return state, fmt.Errorf("备份包含无效页面路径: %s", name)
			}
			target = filepath.Join(stage, "runtime/root", filepath.FromSlash(relative))
		} else if len(parts) < 2 || parts[0] != "sites" || !domain.ValidID(parts[1]) {
			return state, fmt.Errorf("备份包含未知路径: %s", name)
		}
		if header.Typeflag == tar.TypeDir {
			if e = os.MkdirAll(target, 0700); e != nil {
				return state, e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return state, e
		}
		file, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return state, e
		}
		_, e = io.CopyN(file, tr, header.Size)
		closeErr := file.Close()
		if e != nil {
			return state, e
		}
		if closeErr != nil {
			return state, closeErr
		}
	}
	// Drain bounded tar padding through the gzip checksum. A second gzip member
	// or hidden nonzero tar payload must never be silently ignored.
	padding, err := io.ReadAll(io.LimitReader(gz, (1<<20)+1))
	if err != nil {
		return state, err
	}
	if len(padding) > 1<<20 || len(bytes.Trim(padding, "\x00")) != 0 {
		return state, fmt.Errorf("备份包含无效 tar 尾部数据")
	}
	if _, err = compressed.Peek(1); err != io.EOF {
		return state, fmt.Errorf("备份包含额外压缩数据")
	}
	if !foundState {
		return state, fmt.Errorf("备份缺少 state.json")
	}
	if legacyExport {
		err = errorpages.New(stage).Ensure(legacyPages)
	} else {
		err = errorpages.New(stage).MarkInitialized()
	}
	if err != nil {
		return state, err
	}
	domain.NormalizeState(&state)
	if err = domain.Validate(state); err != nil {
		return state, err
	}
	validSites := map[string]bool{}
	for _, s := range state.Sites {
		validSites[s.ID] = true
	}
	entries, err := os.ReadDir(filepath.Join(stage, "sites"))
	if err != nil {
		return state, err
	}
	for _, e := range entries {
		if !validSites[e.Name()] {
			return state, fmt.Errorf("备份存在未登记网站目录")
		}
	}
	pageSites, err := os.ReadDir(filepath.Join(stage, "runtime/root/sites"))
	if err != nil {
		return state, err
	}
	for _, entry := range pageSites {
		if !validSites[entry.Name()] {
			return state, fmt.Errorf("备份存在未登记网站页面目录")
		}
	}
	for _, scope := range append([]string{""}, siteIDs(state.Sites)...) {
		document, e := errorpages.New(stage).Read(scope)
		if e != nil {
			return state, e
		}
		if e = domain.ValidatePages(document.Pages); e != nil {
			return state, e
		}
	}
	return state, nil
}

func siteIDs(sites []domain.Site) []string {
	ids := make([]string, 0, len(sites))
	for _, site := range sites {
		ids = append(ids, site.ID)
	}
	return ids
}

func Verify(reader io.Reader, expected string) error {
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(reader, MaxArchiveSize+1)); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return fmt.Errorf("备份 SHA-256 校验失败")
	}
	return nil
}
