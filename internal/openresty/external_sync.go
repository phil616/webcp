package openresty

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"web-panel/internal/domain"
)

// SyncFile keeps the standalone site's instant file edits and HTTP-01 tokens
// working even when an existing container has no data bind mounts.
func (c *Controller) SyncFile(ctx context.Context, kind, site, name string, remove bool) error {
	c.op.Lock()
	defer c.op.Unlock()
	value := c.Connection()
	if value.Mode != "existing" || value.BoundData {
		return nil
	}
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("无效同步路径")
	}
	var host, container string
	if kind == "sites" && domain.ValidID(site) {
		host = filepath.Join(c.root, "sites", site, filepath.FromSlash(name))
		container = path.Join(Mount, "sites", site, name)
	} else if kind == "acme" {
		host = filepath.Join(c.root, "runtime/acme", filepath.FromSlash(name))
		container = path.Join(Mount, "acme", name)
	} else {
		return fmt.Errorf("不支持的同步类型")
	}
	if _, err := c.selectedExisting(ctx, value); err != nil {
		return err
	}
	if remove {
		_, err := c.externalExec(ctx, value, "rm", "-f", "--", container)
		return err
	}
	if _, err := c.externalExec(ctx, value, "mkdir", "-p", path.Dir(container)); err != nil {
		return err
	}
	temporary := container + ".webcp-" + domain.NewID()
	if err := c.copyInto(ctx, value, host, temporary); err != nil {
		return err
	}
	if _, err := c.externalExec(ctx, value, "chown", value.WorkerUID+":"+value.WorkerGID, temporary); err != nil {
		return err
	}
	_, err := c.externalExec(ctx, value, "mv", "-f", temporary, container)
	return err
}

// SyncPages transports engine assets only for unmounted existing containers.
// Stopped and not-yet-adopted containers receive them at the next deployment.
func (c *Controller) SyncPages(ctx context.Context) error {
	c.op.Lock()
	defer c.op.Unlock()
	value := c.Connection()
	if value.Mode != "existing" || value.BoundData {
		return nil
	}
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return err
	}
	if !info.State.Running {
		return nil
	}
	marker, err := c.externalExec(ctx, value, "cat", Mount+"/.webcp-owner")
	if err != nil || strings.TrimSpace(marker) != c.owner() {
		return nil
	}
	return c.syncPageFiles(ctx, value)
}
