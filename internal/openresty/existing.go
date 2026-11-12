package openresty

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func (c *Controller) applyExisting(ctx context.Context, value Connection, g Generation) error {
	probe, err := c.Probe(ctx, value)
	if err != nil {
		return err
	}
	if !probe.Compatible {
		return fmt.Errorf("所选容器不兼容: %v", probe.Problems)
	}
	d := c.docker(value)
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return err
	}
	desired, err := d.bindings(g.Ports, g.WebPorts...)
	if err != nil {
		return err
	}
	actual, err := publishedBindings(info)
	if err != nil {
		return err
	}
	// Externally managed containers retain their lifecycle and port publications.
	for _, required := range desired {
		if !slices.ContainsFunc(actual, func(port PortBinding) bool {
			return port.Protocol == required.Protocol && port.ContainerPort == required.ContainerPort && port.HostPort == required.HostPort && port.HostAddress == required.HostAddress
		}) {
			return fmt.Errorf("已有容器缺少端口映射 %d:%d/%s，请在 Docker 配置中添加", required.HostPort, required.ContainerPort, required.Protocol)
		}
	}
	candidate := Mount + "/generations/" + g.ID + "/nginx.conf"
	validate := func(binary string, arguments ...string) error {
		var args []string
		if info.State.Running {
			args = append([]string{"exec", info.ID, binary}, arguments...)
		} else {
			args = append([]string{"run", "--rm", "--entrypoint", binary}, d.mountArgs()...)
			args = append(args, info.Config.Image)
			args = append(args, arguments...)
		}
		_, err := d.run(ctx, args...)
		return err
	}
	if err = validate("/usr/local/openresty/bin/openresty", "-t", "-c", candidate); err != nil {
		return err
	}
	for _, folder := range []string{"modules", "blocks"} {
		files, e := filepath.Glob(filepath.Join(g.Path, folder, "*.lua"))
		if e != nil {
			return e
		}
		for _, file := range files {
			if e = validate("/usr/local/openresty/luajit/bin/luajit", "-b", Mount+"/generations/"+g.ID+"/"+folder+"/"+filepath.Base(file), "/dev/null"); e != nil {
				return e
			}
		}
	}

	old, _ := os.Readlink(filepath.Join(c.root, "runtime/current"))
	if err = d.link("generations/" + g.ID); err != nil {
		return err
	}
	var args []string
	if info.State.Running {
		args = []string{"exec", info.ID, "/usr/local/openresty/bin/openresty", "-s", "reload", "-c", Mount + "/current/nginx.conf"}
	} else {
		args = []string{"start", info.ID}
	}
	if _, err = d.run(ctx, args...); err != nil {
		if old != "" {
			_ = d.link(old)
		} else {
			_ = os.Remove(filepath.Join(c.root, "runtime/current"))
		}
		return err
	}
	return nil
}
