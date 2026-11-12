package openresty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"web-panel/internal/domain"
)

type Status struct {
	Available       bool          `json:"available"`
	Running         bool          `json:"running"`
	Container       string        `json:"container"`
	Image           string        `json:"image"`
	AppliedRevision uint64        `json:"appliedRevision"`
	Detail          string        `json:"detail"`
	PublishedPorts  []PortBinding `json:"publishedPorts"`
}
type Runtime interface {
	Status(context.Context) Status
	Apply(context.Context, Generation) error
	Stop(context.Context) error
	Logs(context.Context) (string, error)
}

type Docker struct{ Root, Image, Container, HTTPBind, HTTPSBind string }

func (d *Docker) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// exec argv, without a shell; credentials never appear in Docker arguments.
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
func (d *Docker) Status(ctx context.Context) Status {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s := Status{Container: d.Container, Image: d.Image, PublishedPorts: []PortBinding{}}
	if _, err := d.run(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		s.Detail = err.Error()
		return s
	}
	s.Available = true
	out, err := d.run(ctx, "inspect", "--format", "{{json .}}", d.Container)
	if err != nil {
		s.Detail = "尚未创建托管容器"
		return s
	}
	var info containerInfo
	if err = json.Unmarshal([]byte(out), &info); err != nil {
		s.Detail = err.Error()
		return s
	}
	if info.Config.Labels["resty.manager.root"] != d.Root {
		s.Detail = "同名容器不属于此数据目录"
		return s
	}
	s.Running = info.State.Running
	s.Image = info.Config.Image
	if ports, e := publishedBindings(&info); e == nil {
		s.PublishedPorts = ports
	} else {
		s.Detail = e.Error()
	}
	b, _ := os.ReadFile(filepath.Join(d.Root, "runtime", "current", "revision"))
	s.AppliedRevision, _ = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return s
}
func (d *Docker) owned(ctx context.Context) (exists, running bool, err error) {
	info, err := d.currentContainer(ctx)
	if err != nil {
		return false, false, err
	}
	if info == nil {
		return false, false, nil
	}
	return true, info.State.Running, nil
}
func (d *Docker) mountArgs() []string {
	return []string{"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src=" + filepath.Join(d.Root, "runtime") + ",dst=" + Mount + ",readonly", "--mount", "type=bind,src=" + filepath.Join(d.Root, "sites") + ",dst=" + Mount + "/sites,readonly", "--mount", "type=bind,src=" + filepath.Join(d.Root, "cache") + ",dst=" + Mount + "/cache"}
}
func (d *Docker) link(target string) error {
	tmp := filepath.Join(d.Root, "runtime", ".current-"+domain.NewID())
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)
	return os.Rename(tmp, filepath.Join(d.Root, "runtime", "current"))
}
func (d *Docker) Apply(ctx context.Context, g Generation) error {
	if err := d.Recover(ctx); err != nil {
		return err
	}
	info, err := d.currentContainer(ctx)
	if err != nil {
		return err
	}
	desired, err := d.bindings(g.Ports, g.WebPorts...)
	if err != nil {
		return err
	}
	running := info != nil && info.State.Running
	replace := info == nil
	if info != nil {
		actual, e := publishedBindings(info)
		if e != nil {
			return e
		}
		replace = info.Config.Labels["resty.manager.spec"] != containerSpec || !slices.Equal(actual, desired)
	}
	candidate := Mount + "/generations/" + g.ID + "/nginx.conf"
	var args []string
	if running && !replace {
		args = []string{"exec", d.Container, "/usr/local/openresty/bin/openresty", "-t", "-c", candidate}
	} else {
		args = append([]string{"run", "--rm", "--add-host", "host.docker.internal:host-gateway", "--entrypoint", "/usr/local/openresty/bin/openresty"}, d.mountArgs()...)
		args = append(args, d.Image, "-t", "-c", candidate)
	}
	if _, err = d.run(ctx, args...); err != nil {
		return fmt.Errorf("OpenResty 配置校验失败: %w", err)
	}
	// nginx -t does not compile *_by_lua_file. Compile every module and block before switching.
	for _, folder := range []string{"modules", "blocks"} {
		files, e := filepath.Glob(filepath.Join(g.Path, folder, "*.lua"))
		if e != nil {
			return e
		}
		for _, file := range files {
			containerFile := Mount + "/generations/" + g.ID + "/" + folder + "/" + filepath.Base(file)
			if running && !replace {
				args = []string{"exec", d.Container, "/usr/local/openresty/luajit/bin/luajit", "-b", containerFile, "/dev/null"}
			} else {
				args = append([]string{"run", "--rm", "--entrypoint", "/usr/local/openresty/luajit/bin/luajit"}, d.mountArgs()...)
				args = append(args, d.Image, "-b", containerFile, "/dev/null")
			}
			if _, e = d.run(ctx, args...); e != nil {
				return fmt.Errorf("lua 编译失败: %w", e)
			}
		}
	}
	if replace {
		return d.replace(ctx, g, info, desired)
	}
	old, _ := os.Readlink(filepath.Join(d.Root, "runtime", "current"))
	if err = d.link("generations/" + g.ID); err != nil {
		return err
	}
	rollback := func(cause error) error {
		if old != "" {
			if e := d.link(old); e != nil {
				return errors.Join(cause, e)
			}
		} else {
			os.Remove(filepath.Join(d.Root, "runtime", "current"))
		}
		return cause
	}
	if running {
		_, err = d.run(ctx, "exec", d.Container, "/usr/local/openresty/bin/openresty", "-s", "reload", "-c", Mount+"/current/nginx.conf")
		if err != nil {
			rollbackErr := rollback(err)
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			_, reloadErr := d.run(cleanup, "exec", d.Container, "/usr/local/openresty/bin/openresty", "-s", "reload", "-c", Mount+"/current/nginx.conf")
			return errors.Join(rollbackErr, reloadErr)
		}
		return nil
	}
	_, err = d.run(ctx, "start", d.Container)
	if err != nil {
		return rollback(err)
	}
	// A container can exit immediately after docker start succeeds.
	if _, isRunning, e := d.owned(ctx); e != nil || !isRunning {
		if e == nil {
			e = fmt.Errorf("OpenResty 启动后退出，请检查日志")
		}
		return rollback(e)
	}
	return nil
}
func (d *Docker) Stop(ctx context.Context) error {
	exists, running, err := d.owned(ctx)
	if err != nil {
		return err
	}
	if !exists || !running {
		return nil
	}
	_, err = d.run(ctx, "stop", "--time", "10", d.Container)
	return err
}
func (d *Docker) Logs(ctx context.Context) (string, error) {
	exists, _, err := d.owned(ctx)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	return d.run(ctx, "logs", "--tail", "200", d.Container)
}

func (d *Docker) ClearProxyCache(ctx context.Context, id string) error {
	if !domain.ValidID(id) {
		return fmt.Errorf("invalid website ID")
	}
	exists, running, err := d.owned(ctx)
	if err != nil {
		return err
	}
	if !exists || !running {
		return fmt.Errorf("请先启动 OpenResty")
	}
	// Removing files preserves nginx's cache directory and never traverses user paths.
	_, err = d.run(ctx, "exec", d.Container, "find", Mount+"/cache/"+id, "-type", "f", "-delete")
	return err
}
