package openresty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/store"
)

// Journal identities are Docker IDs, not names: rollback must never accidentally
// remove the old or an unrelated container after an interrupted rename.
type replacementJournal struct {
	Container  string `json:"container"`
	OldID      string `json:"oldId"`
	NewID      string `json:"newId"`
	OldTarget  string `json:"oldTarget"`
	OldRunning bool   `json:"oldRunning"`
}

func (d *Docker) journalPath() string {
	return filepath.Join(d.Root, "runtime", "replace-journal.json")
}
func validTarget(target string) bool {
	return target == "" || (strings.HasPrefix(target, "generations/") && domain.ValidID(strings.TrimPrefix(target, "generations/")))
}

func (d *Docker) existingByID(ctx context.Context, id string) (*containerInfo, error) {
	if id == "" {
		return nil, nil
	}
	out, err := d.run(ctx, "container", "ls", "-a", "--filter", "id="+id, "--format", "{{.ID}}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return d.inspectOwned(ctx, id)
}

func (d *Docker) rollbackReplacement(ctx context.Context, j replacementJournal) error {
	old, err := d.existingByID(ctx, j.OldID)
	if err != nil {
		return err
	}
	if j.OldID != "" && old == nil {
		return fmt.Errorf("恢复旧容器失败：原容器 %s 已不存在，保留恢复日志", j.OldID)
	}
	newContainer, err := d.existingByID(ctx, j.NewID)
	if err != nil {
		return err
	}
	if newContainer != nil {
		if _, err = d.run(ctx, "rm", "-f", j.NewID); err != nil {
			return err
		}
	}
	if old != nil && old.Name != "/"+d.Container {
		if _, err = d.run(ctx, "rename", j.OldID, d.Container); err != nil {
			return err
		}
	}
	if j.OldTarget != "" {
		if err = d.link(j.OldTarget); err != nil {
			return err
		}
	} else if err = os.Remove(filepath.Join(d.Root, "runtime", "current")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if old != nil && j.OldRunning && !old.State.Running {
		if _, err = d.run(ctx, "start", j.OldID); err != nil {
			return err
		}
	}
	return os.Remove(d.journalPath())
}

// Recover is called when opening the manager and before deployments. A journal
// remains until either the replacement commits or rollback completes.
func (d *Docker) Recover(ctx context.Context) error {
	data, err := os.ReadFile(d.journalPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) > 64<<10 {
		return fmt.Errorf("无效容器切换日志")
	}
	var journal replacementJournal
	if err = json.Unmarshal(data, &journal); err != nil {
		return err
	}
	id := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if journal.Container != d.Container || !id.MatchString(journal.NewID) || (journal.OldID != "" && !id.MatchString(journal.OldID)) || journal.OldID == journal.NewID || !validTarget(journal.OldTarget) {
		return fmt.Errorf("无效容器切换日志")
	}
	if err = d.rollbackReplacement(ctx, journal); err != nil {
		return fmt.Errorf("端口发布切换恢复失败: %w", err)
	}
	return nil
}

func (d *Docker) replace(ctx context.Context, g Generation, old *containerInfo, ports []PortBinding) error {
	target, err := os.Readlink(filepath.Join(d.Root, "runtime", "current"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !validTarget(target) || (old != nil && old.State.Running && target == "") {
		return fmt.Errorf("当前部署链接无效，拒绝切换容器")
	}
	candidateName := d.Container + "-candidate-" + g.ID[:12]
	id, err := d.createContainer(ctx, candidateName, ports)
	if err != nil {
		return err
	}
	journal := replacementJournal{Container: d.Container, NewID: id, OldTarget: target}
	if old != nil {
		journal.OldID = old.ID
		journal.OldRunning = old.State.Running
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if err = store.AtomicWrite(d.journalPath(), data, 0600); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, removeErr := d.run(cleanup, "rm", "-f", id)
		return errors.Join(err, removeErr)
	}
	rollback := func(cause error) error {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		if e := d.rollbackReplacement(cleanup, journal); e != nil {
			return errors.Join(cause, fmt.Errorf("回滚失败，恢复日志已保留: %w", e))
		}
		return cause
	}
	if old != nil {
		if old.State.Running {
			if _, err = d.run(ctx, "stop", "--time", "5", old.ID); err != nil {
				return rollback(err)
			}
		}
		if _, err = d.run(ctx, "rename", old.ID, d.Container+"-rollback-"+g.ID[:12]); err != nil {
			return rollback(err)
		}
	}
	if err = d.link("generations/" + g.ID); err != nil {
		return rollback(err)
	}
	if _, err = d.run(ctx, "rename", id, d.Container); err != nil {
		return rollback(err)
	}
	if _, err = d.run(ctx, "start", id); err != nil {
		return rollback(err)
	}
	if _, err = d.run(ctx, "exec", id, "/usr/local/openresty/bin/openresty", "-t", "-c", Mount+"/current/nginx.conf"); err != nil {
		return rollback(err)
	}
	info, err := d.inspectOwned(ctx, id)
	if err != nil {
		return rollback(err)
	}
	if !info.State.Running {
		return rollback(fmt.Errorf("新 OpenResty 容器启动后退出"))
	}
	if err = os.Remove(d.journalPath()); err != nil {
		return rollback(err)
	}
	if old != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err = d.run(cleanup, "rm", old.ID); err != nil {
			slog.Warn("部署已提交，但清理旧托管容器失败", "container", old.ID, "error", err)
		}
	}
	return nil
}
