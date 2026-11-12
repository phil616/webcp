package openresty

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
	"web-panel/internal/domain"
)

func (c *Controller) externalExec(ctx context.Context, value Connection, args ...string) (string, error) {
	return c.docker(value).run(ctx, append([]string{"exec", "--user", "0", value.ContainerID}, args...)...)
}

func (c *Controller) masterExec(ctx context.Context, value Connection, arguments ...string) (string, error) {
	uid := value.MasterUID
	if uid == "" {
		uid = "0"
	}
	return c.docker(value).run(ctx, append([]string{"exec", "--user", uid, value.ContainerID}, arguments...)...)
}

func (c *Controller) copyInto(ctx context.Context, value Connection, source, target string) error {
	_, err := c.docker(value).run(ctx, "cp", source, value.ContainerID+":"+target)
	return err
}

func (c *Controller) containerLink(ctx context.Context, value Connection, target, name string) error {
	temporary := name + ".new-" + domain.NewID()
	_, err := c.externalExec(ctx, value, "/bin/sh", "-c", `ln -s "$1" "$2" && mv -Tf "$2" "$3"`, "--", target, temporary, name)
	return err
}

func (c *Controller) syncDirectory(ctx context.Context, value Connection, source, target string) error {
	if _, err := c.externalExec(ctx, value, "mkdir", "-p", target); err != nil {
		return err
	}
	if err := c.copyInto(ctx, value, source+"/.", target); err != nil {
		return err
	}
	_, err := c.externalExec(ctx, value, "chown", "-R", value.WorkerUID+":"+value.WorkerGID, target)
	return err
}

func (c *Controller) owner() string {
	digest := sha256.Sum256([]byte(c.root))
	return hex.EncodeToString(digest[:])
}

func (c *Controller) externalDir(value Connection) string {
	return filepath.Join(c.root, "runtime", "external", value.ContainerID)
}

// Keep nginx -T's diagnostics out of its parseable configuration output.
func (c *Controller) masterOutput(ctx context.Context, value Connection, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	uid := value.MasterUID
	if uid == "" {
		uid = "0"
	}
	command := exec.CommandContext(ctx, "docker", append([]string{"exec", "--user", uid, value.ContainerID}, arguments...)...)
	var output, diagnostic bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostic
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("读取容器主配置失败: %w: %s", err, diagnostic.String())
	}
	return output.String(), nil
}
