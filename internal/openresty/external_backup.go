package openresty

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"web-panel/internal/domain"
	"web-panel/internal/store"
)

func (c *Controller) backupOriginal(ctx context.Context, value Connection) error {
	folder := c.externalDir(value)
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	file := filepath.Join(folder, "original-nginx.conf")
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	temporary := file + ".tmp"
	defer os.Remove(temporary)
	if _, err := c.docker(value).run(ctx, "cp", "-L", value.ContainerID+":"+value.MainConfig, temporary); err != nil {
		return fmt.Errorf("备份原主配置失败: %w", err)
	}
	if err := os.Chmod(temporary, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, file)
}

type takeoverJournal struct {
	HostTarget    string `json:"hostTarget"`
	OldRevision   string `json:"oldRevision"`
	Previous      string `json:"previous"`
	CurrentTarget string `json:"currentTarget"`
	SitesTarget   string `json:"sitesTarget"`
}

func (c *Controller) journalTakeover(value Connection) string {
	return filepath.Join(c.externalDir(value), "apply-journal.json")
}

func (c *Controller) recoverTakeover(ctx context.Context, value Connection) error {
	data, err := os.ReadFile(c.journalTakeover(value))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal takeoverJournal
	if err = json.Unmarshal(data, &journal); err != nil {
		return err
	}
	if err = c.rollbackTakeover(ctx, value, journal); err != nil {
		return err
	}
	return os.Remove(c.journalTakeover(value))
}

func (c *Controller) rollbackTakeover(ctx context.Context, value Connection, journal takeoverJournal) error {
	if journal.Previous == "" || filepath.Dir(journal.Previous) != c.externalDir(value) || filepath.Clean(journal.Previous) != journal.Previous || !validTarget(journal.HostTarget) || !validTarget(journal.CurrentTarget) || (journal.SitesTarget != "" && (!strings.HasPrefix(journal.SitesTarget, "site-versions/") || !domain.ValidID(strings.TrimPrefix(journal.SitesTarget, "site-versions/")))) {
		return fmt.Errorf("无效主配置恢复日志")
	}
	temporary := "/tmp/webcp-rollback-" + domain.NewID() + ".conf"
	if err := c.copyInto(ctx, value, journal.Previous, temporary); err != nil {
		return err
	}
	if _, err := c.externalExec(ctx, value, "cp", temporary, value.MainConfig); err != nil {
		return err
	}
	if journal.CurrentTarget != "" {
		if value.BoundData {
			if err := c.docker(value).link(journal.CurrentTarget); err != nil {
				return err
			}
		} else if err := c.containerLink(ctx, value, journal.CurrentTarget, Mount+"/current"); err != nil {
			return err
		}
	}
	if !value.BoundData && journal.SitesTarget != "" {
		if err := c.containerLink(ctx, value, journal.SitesTarget, Mount+"/sites"); err != nil {
			return err
		}
	}
	if !value.BoundData {
		if journal.HostTarget != "" {
			if err := c.docker(value).link(journal.HostTarget); err != nil {
				return err
			}
		} else {
			_ = os.Remove(filepath.Join(c.root, "runtime/current"))
		}
	}
	if journal.OldRevision != "" {
		if err := store.AtomicWrite(filepath.Join(c.externalDir(value), "revision"), []byte(journal.OldRevision), 0600); err != nil {
			return err
		}
	} else {
		_ = os.Remove(filepath.Join(c.externalDir(value), "revision"))
	}
	_, _, _, err := c.master(ctx, value)
	if err == nil {
		err = c.signalMaster(ctx, value)
	}
	return err
}
