package openresty

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"web-panel/internal/domain"
)

const containerSpec = "3" // Includes explicit dynamic publication and host-gateway support.

type hostPort struct {
	HostIP   string
	HostPort string
}
type containerInfo struct {
	ID     string
	Name   string
	State  struct{ Running bool }
	Config struct {
		Image      string
		User       string
		Cmd        []string
		Entrypoint []string
		Labels     map[string]string
	}
	HostConfig struct{ PortBindings map[string][]hostPort }
	Mounts     []struct {
		Source      string
		Destination string
		RW          bool
	}
}

func (d *Docker) inspectOwned(ctx context.Context, ref string) (*containerInfo, error) {
	out, err := d.run(ctx, "inspect", "--format", "{{json .}}", ref)
	if err != nil {
		return nil, err
	}
	var info containerInfo
	if err = json.Unmarshal([]byte(out), &info); err != nil {
		return nil, err
	}
	if info.Config.Labels["resty.manager.root"] != d.Root {
		return nil, fmt.Errorf("拒绝操作不属于此数据目录的容器 %s", ref)
	}
	return &info, nil
}

func (d *Docker) currentContainer(ctx context.Context) (*containerInfo, error) {
	out, err := d.run(ctx, "container", "ls", "-a", "--filter", "name=^/"+regexp.QuoteMeta(d.Container)+"$", "--format", "{{.ID}}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return d.inspectOwned(ctx, d.Container)
}

func normalizeBinding(binding PortBinding) (PortBinding, error) {
	if binding.HostAddress == "" {
		binding.HostAddress = "0.0.0.0"
	}
	ip := net.ParseIP(binding.HostAddress)
	if ip == nil || binding.HostPort < 1 || binding.HostPort > 65535 || binding.ContainerPort < 1 || binding.ContainerPort > 65535 || (binding.Protocol != "tcp" && binding.Protocol != "udp") {
		return binding, fmt.Errorf("无效容器端口发布参数")
	}
	binding.HostAddress = ip.String()
	return binding, nil
}

func (d *Docker) bindings(streams []PortBinding, web ...WebsitePort) ([]PortBinding, error) {
	ports := make([]PortBinding, 0, len(streams)+2)
	for _, base := range []struct {
		bind string
		port int
	}{{d.HTTPBind, 80}, {d.HTTPSBind, 443}} {
		host, port, err := net.SplitHostPort(base.bind)
		if err != nil {
			return nil, fmt.Errorf("网站发布地址需为 IP:端口: %w", err)
		}
		number, err := strconv.Atoi(port)
		if err != nil {
			return nil, err
		}
		ports = append(ports, PortBinding{HostAddress: host, HostPort: number, ContainerPort: base.port, Protocol: "tcp"})
	}
	website, err := d.websiteBindings(web)
	if err != nil {
		return nil, err
	}
	ports = append(ports, website...)
	ports = append(ports, streams...)
	for i := range ports {
		var err error
		ports[i], err = normalizeBinding(ports[i])
		if err != nil {
			return nil, err
		}
		for _, other := range ports[:i] {
			if ports[i].Protocol != other.Protocol {
				continue
			}
			if ports[i].ContainerPort == other.ContainerPort {
				return nil, fmt.Errorf("容器内部监听端口冲突")
			}
			if domain.ListenerOverlap(ports[i].HostAddress, ports[i].HostPort, other.HostAddress, other.HostPort) {
				return nil, fmt.Errorf("监听地址 %s 与网站或其他四层代理的发布端口冲突", net.JoinHostPort(ports[i].HostAddress, strconv.Itoa(ports[i].HostPort)))
			}
		}
	}
	sortBindings(ports)
	return ports, nil
}

func sortBindings(ports []PortBinding) {
	slices.SortFunc(ports, func(a, b PortBinding) int { return strings.Compare(bindingKey(a), bindingKey(b)) })
}
func bindingKey(p PortBinding) string {
	return fmt.Sprintf("%s/%05d/%s/%05d", p.Protocol, p.HostPort, p.HostAddress, p.ContainerPort)
}

func publishedBindings(info *containerInfo) ([]PortBinding, error) {
	ports := []PortBinding{}
	for key, bindings := range info.HostConfig.PortBindings {
		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效容器端口信息")
		}
		containerPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil {
				return nil, err
			}
			p, err := normalizeBinding(PortBinding{HostAddress: binding.HostIP, HostPort: hostPort, ContainerPort: containerPort, Protocol: parts[1]})
			if err != nil {
				return nil, err
			}
			ports = append(ports, p)
		}
	}
	sortBindings(ports)
	return ports, nil
}

// ValidateStreams checks CLI website publications too, without contacting Docker.
func (d *Docker) ValidateStreams(streams []domain.StreamProxy) error {
	var scratch strings.Builder
	_, err := d.bindings(renderStreams(&scratch, streams))
	return err
}

func (d *Docker) createContainer(ctx context.Context, name string, ports []PortBinding) (string, error) {
	args := []string{"create", "--name", name, "--label", "resty.manager.root=" + d.Root, "--label", "resty.manager.spec=" + containerSpec, "--restart", "unless-stopped", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=128m", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--add-host", "host.docker.internal:host-gateway", "--entrypoint", "/usr/local/openresty/bin/openresty"}
	for _, p := range ports {
		args = append(args, "--publish", net.JoinHostPort(p.HostAddress, strconv.Itoa(p.HostPort))+":"+strconv.Itoa(p.ContainerPort)+"/"+p.Protocol)
	}
	args = append(args, d.mountArgs()...)
	args = append(args, d.Image, "-c", Mount+"/current/nginx.conf", "-g", "daemon off;")
	_, err := d.run(ctx, args...)
	if err != nil {
		return "", err
	}
	// Inspect returns the canonical ID even when docker create emitted warnings.
	info, err := d.inspectOwned(ctx, name)
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
