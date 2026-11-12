package openresty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"web-panel/internal/store"
)

func (c *Controller) applyTakeover(ctx context.Context, value Connection, g Generation) (err error) {
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return err
	}
	if !info.State.Running {
		if _, err = c.docker(value).run(ctx, "start", info.ID); err != nil {
			return err
		}
	}
	checked, _, e := c.discoverExternal(ctx, value)
	if e != nil {
		return e
	}
	if checked.WorkerUID != value.WorkerUID || checked.WorkerUser != value.WorkerUser || checked.MasterUID != value.MasterUID || checked.BoundData != value.BoundData {
		return fmt.Errorf("容器运行设置已变化，请重新检测并保存连接")
	}
	if err = c.recoverTakeover(ctx, value); err != nil {
		return err
	}
	if value.BoundData && value.MainConfig == Mount+"/current/nginx.conf" {
		if err = c.backupOriginal(ctx, value); err != nil {
			return err
		}
		return c.applyExisting(ctx, value, g)
	}
	if err = c.backupOriginal(ctx, value); err != nil {
		return err
	}
	desired, err := c.docker(value).bindings(g.Ports, g.WebPorts...)
	if err != nil {
		return err
	}
	actual, err := publishedBindings(info)
	if err != nil {
		return err
	}
	for _, required := range desired {
		if (required.ContainerPort == 80 || required.ContainerPort == 443) && required.Protocol == "tcp" && !slices.Contains(g.ActiveHTTPPorts, required.ContainerPort) {
			continue
		}
		if !slices.ContainsFunc(actual, func(p PortBinding) bool {
			return p.Protocol == required.Protocol && p.ContainerPort == required.ContainerPort && p.HostPort == required.HostPort && p.HostAddress == required.HostAddress
		}) {
			return fmt.Errorf("已有容器缺少端口映射 %d:%d/%s，请先在 Docker 中添加映射", required.HostPort, required.ContainerPort, required.Protocol)
		}
	}
	folder := c.externalDir(value)
	previous := filepath.Join(folder, "before-"+g.ID+".conf")
	if _, err = c.docker(value).run(ctx, "cp", "-L", value.ContainerID+":"+value.MainConfig, previous); err != nil {
		return err
	}
	if err = os.Chmod(previous, 0600); err != nil {
		return err
	}
	journal := takeoverJournal{Previous: previous}
	journal.HostTarget, _ = os.Readlink(filepath.Join(c.root, "runtime/current"))
	revisionData, _ := os.ReadFile(filepath.Join(folder, "revision"))
	journal.OldRevision = string(revisionData)
	if value.BoundData {
		journal.CurrentTarget, _ = os.Readlink(filepath.Join(c.root, "runtime/current"))
	} else {
		text, _ := c.externalExec(ctx, value, "readlink", Mount+"/current")
		journal.CurrentTarget = strings.TrimSpace(text)
		text, _ = c.externalExec(ctx, value, "readlink", Mount+"/sites")
		journal.SitesTarget = strings.TrimSpace(text)
		if _, err = c.externalExec(ctx, value, "mkdir", "-p", Mount+"/generations", Mount+"/site-versions", Mount+"/cache", Mount+"/acme"); err != nil {
			return err
		}
		marker := filepath.Join(folder, "owner")
		if err = os.WriteFile(marker, []byte(c.owner()), 0600); err != nil {
			return err
		}
		if err = c.copyInto(ctx, value, marker, Mount+"/.webcp-owner"); err != nil {
			return err
		}
		if err = c.copyInto(ctx, value, g.Path, Mount+"/generations/"); err != nil {
			return err
		}
		if _, err = c.externalExec(ctx, value, "chown", "-R", value.WorkerUID+":"+value.WorkerGID, Mount+"/generations/"+g.ID, Mount+"/cache"); err != nil {
			return err
		}
		if err = c.syncDirectory(ctx, value, filepath.Join(c.root, "sites"), Mount+"/site-versions/"+g.ID); err != nil {
			return err
		}
		if err = c.syncPageFiles(ctx, value); err != nil {
			return err
		}
		if err = c.syncDirectory(ctx, value, filepath.Join(c.root, "runtime/acme"), Mount+"/acme"); err != nil {
			return err
		}
	}
	candidate := Mount + "/generations/" + g.ID + "/nginx.conf"
	// Validate before touching the original configuration or active links.
	if _, err = c.masterExec(ctx, value, value.Binary, "-t", "-c", candidate); err != nil {
		return fmt.Errorf("OpenResty 配置校验失败: %w", err)
	}
	for _, kind := range []string{"modules", "blocks"} {
		files, e := filepath.Glob(filepath.Join(g.Path, kind, "*.lua"))
		if e != nil {
			return e
		}
		for _, file := range files {
			if _, e = c.masterExec(ctx, value, value.LuaJIT, "-b", Mount+"/generations/"+g.ID+"/"+kind+"/"+filepath.Base(file), "/dev/null"); e != nil {
				return e
			}
		}
	}
	encoded, _ := json.Marshal(journal)
	if err = store.AtomicWrite(c.journalTakeover(value), encoded, 0600); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if rollbackErr := c.rollbackTakeover(cleanup, value, journal); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("恢复原配置失败，日志已保留: %w", rollbackErr))
			} else {
				_ = os.Remove(c.journalTakeover(value))
			}
		}
	}()
	if value.BoundData {
		err = c.docker(value).link("generations/" + g.ID)
	} else {
		err = c.containerLink(ctx, value, "generations/"+g.ID, Mount+"/current")
		if err == nil {
			err = c.containerLink(ctx, value, "site-versions/"+g.ID, Mount+"/sites")
		}
	}
	if err != nil {
		return err
	}
	wrapper := filepath.Join(folder, "webcp-main.conf")
	if err = store.AtomicWrite(wrapper, []byte("# Managed by WebCP. Original configuration is backed up by container ID.\ninclude "+Mount+"/current/nginx.conf;\n"), 0600); err != nil {
		return err
	}
	temporary := "/tmp/webcp-main-" + g.ID + ".conf"
	if err = c.copyInto(ctx, value, wrapper, temporary); err != nil {
		return err
	}
	if _, err = c.externalExec(ctx, value, "cp", temporary, value.MainConfig); err != nil {
		return err
	}
	if _, err = c.masterExec(ctx, value, value.Binary, "-t", "-c", value.MainConfig); err != nil {
		return err
	}
	if err = c.signalMaster(ctx, value); err != nil {
		return err
	}
	if !value.BoundData {
		if err = c.docker(value).link("generations/" + g.ID); err != nil {
			return err
		}
	}
	if err = store.AtomicWrite(filepath.Join(folder, "revision"), []byte(fmt.Sprint(g.Revision)), 0600); err != nil {
		return err
	}
	if err = os.Remove(c.journalTakeover(value)); err != nil {
		return err
	}
	committed = true
	return nil
}

func (c *Controller) signalMaster(ctx context.Context, value Connection) error {
	pid, _, _, err := c.master(ctx, value)
	if err != nil {
		return err
	}
	_, err = c.masterExec(ctx, value, "kill", "-HUP", pid)
	return err
}
