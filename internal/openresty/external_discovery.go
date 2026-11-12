package openresty

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"web-panel/internal/onepanel/nginx/components"
	"web-panel/internal/onepanel/nginx/parser"
)

// The script is constant; all selected paths and IDs travel as argv, never shell text.
const masterScript = `for proc in /proc/[0-9]*; do
 [ -r "$proc/cmdline" ] || continue
 cmd=$(tr '\000' ' ' < "$proc/cmdline")
 case "$cmd" in 'nginx: master process '*)
  printf '%s\n' "${proc##*/}"
  awk '/^Uid:/{print $2}' "$proc/status"
  printf '%s\n' "$cmd"
 ;; esac
done`

func safeExecutable(value string) bool {
	return regexp.MustCompile(`^/?[A-Za-z0-9_./-]+$`).MatchString(value) && !strings.HasPrefix(value, "-") && !strings.Contains(value, "..")
}

func safeContainerPath(value string) bool {
	return path.IsAbs(value) && path.Clean(value) == value && !strings.ContainsAny(value, "\x00\r\n\t '\";{}$")
}

func binaryOption(output, key string) string {
	match := regexp.MustCompile(`(?:^|\s)--` + key + `=([^\s'"]+)`).FindStringSubmatch(output)
	if match != nil {
		return match[1]
	}
	return ""
}

func (c *Controller) master(ctx context.Context, value Connection) (pid, uid, command string, err error) {
	output, err := c.externalExec(ctx, value, "/bin/sh", "-c", masterScript)
	if err != nil {
		return "", "", "", err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		return "", "", "", fmt.Errorf("所选容器需有且仅有一个正在运行的 OpenResty 主进程")
	}
	if _, err = strconv.Atoi(lines[0]); err != nil {
		return "", "", "", fmt.Errorf("无效 OpenResty 进程")
	}
	return lines[0], lines[1], lines[2], nil
}

func (c *Controller) discoverExternal(ctx context.Context, value Connection) (Connection, *containerInfo, error) {
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return value, nil, err
	}
	value.ContainerID = info.ID
	value.Image = info.Config.Image
	if !info.State.Running {
		return value, nil, fmt.Errorf("检测或接管配置前请先启动所选容器")
	}

	candidates := []string{value.Binary, "/usr/local/openresty/bin/openresty", "/usr/local/openresty/nginx/sbin/nginx", "openresty", "nginx"}
	version := ""
	for _, binary := range candidates {
		if !safeExecutable(binary) {
			continue
		}
		text, e := c.externalExec(ctx, value, binary, "-V")
		if e == nil && strings.Contains(strings.ToLower(text), "openresty") {
			value.Binary = binary
			version = text
			break
		}
	}
	if version == "" {
		return value, nil, fmt.Errorf("未检测到 OpenResty，请在高级设置填写实际可执行程序路径")
	}
	_, uid, command, err := c.master(ctx, value)
	if err != nil {
		return value, nil, err
	}
	value.MasterUID = uid
	prefix := binaryOption(version, "prefix")
	if prefix == "" {
		prefix = "/usr/local/openresty/nginx"
	}
	if value.MainConfig == "" {
		matches := regexp.MustCompile(`(?:^|\s)-c\s+([^\s;]+)`).FindStringSubmatch(command)
		if matches != nil {
			value.MainConfig = matches[1]
		} else {
			value.MainConfig = binaryOption(version, "conf-path")
			if value.MainConfig == "" {
				value.MainConfig = "conf/nginx.conf"
			}
		}
		if !path.IsAbs(value.MainConfig) {
			value.MainConfig = path.Join(prefix, value.MainConfig)
		}
	}
	actual := binaryOption(version, "conf-path")
	if actual == "" {
		actual = "conf/nginx.conf"
	}
	if match := regexp.MustCompile(`(?:^|\s)-c\s+([^\s;]+)`).FindStringSubmatch(command); match != nil {
		actual = match[1]
	}
	if !path.IsAbs(actual) {
		actual = path.Join(prefix, actual)
	}
	if actual != value.MainConfig {
		a, _ := c.externalExec(ctx, value, "readlink", "-f", actual)
		b, _ := c.externalExec(ctx, value, "readlink", "-f", value.MainConfig)
		if strings.TrimSpace(a) == "" || strings.TrimSpace(a) != strings.TrimSpace(b) {
			return value, nil, fmt.Errorf("指定的主配置与运行进程的配置 %s 不一致", actual)
		}
	}
	if !safeContainerPath(value.MainConfig) {
		return value, nil, fmt.Errorf("主配置路径必须是容器内的绝对路径")
	}
	dump, err := c.masterOutput(ctx, value, value.Binary, "-T", "-c", value.MainConfig)
	if err != nil {
		return value, nil, fmt.Errorf("读取已有主配置失败，请检查主配置路径: %w", err)
	}
	directives, err := globalNginxDirectives(dump)
	if err != nil {
		return value, nil, err
	}
	value.LoadModules = nil
	user := binaryOption(version, "user")
	if user == "" {
		user = "nobody"
	}
	explicitGroup := ""
	for _, directive := range directives.GetDirectives() {
		params := directive.GetParameters()
		if len(params) == 0 {
			continue
		}
		switch directive.GetName() {
		case "load_module":
			module := params[0]
			if !path.IsAbs(module) {
				module = path.Join(prefix, module)
			}
			if !safeContainerPath(module) {
				return value, nil, fmt.Errorf("无效动态模块路径")
			}
			value.LoadModules = append(value.LoadModules, module)
		case "user":
			user = params[0]
			if len(params) > 1 {
				explicitGroup = params[1]
			}
		}
	}
	value.WorkerUser = user
	groupName, e := c.externalExec(ctx, value, "id", "-gn", user)
	if e != nil {
		return value, nil, e
	}
	value.WorkerGroup = strings.TrimSpace(groupName)
	if explicitGroup != "" {
		value.WorkerGroup = explicitGroup
	}
	if uid != "0" {
		value.WorkerUID = uid
		gid, e := c.externalExec(ctx, value, "id", "-g")
		if e != nil {
			return value, nil, e
		}
		value.WorkerGID = strings.TrimSpace(gid)
	} else {
		worker, e := c.externalExec(ctx, value, "id", "-u", user)
		if e != nil {
			return value, nil, e
		}
		group, e := c.externalExec(ctx, value, "id", "-g", user)
		if e != nil {
			return value, nil, e
		}
		value.WorkerUID = strings.TrimSpace(worker)
		value.WorkerGID = strings.TrimSpace(group)
	}
	value.MIMEPath = path.Join(path.Dir(value.MainConfig), "mime.types")
	if _, e := c.externalExec(ctx, value, "test", "-f", value.MIMEPath); e != nil {
		value.MIMEPath = path.Join(prefix, "conf/mime.types")
	}
	value.LuaJIT = strings.TrimSpace(value.LuaJIT)
	luaFound := false
	for _, binary := range []string{value.LuaJIT, "/usr/local/openresty/luajit/bin/luajit", "luajit"} {
		if !safeExecutable(binary) {
			continue
		}
		if text, e := c.externalExec(ctx, value, binary, "-v"); e == nil && strings.Contains(text, "LuaJIT") {
			luaFound = true
			value.LuaJIT = binary
			break
		}
	}
	if !luaFound {
		return value, nil, fmt.Errorf("未检测到 LuaJIT")
	}
	mounts := map[string]string{}
	for _, mount := range info.Mounts {
		mounts[mount.Destination] = mount.Source
	}
	value.BoundData = mounts[Mount] == filepath.Join(c.root, "runtime") && mounts[Mount+"/sites"] == filepath.Join(c.root, "sites") && mounts[Mount+"/cache"] == filepath.Join(c.root, "cache")
	if value.BoundData && value.WorkerUID != fmt.Sprint(os.Getuid()) {
		return value, nil, fmt.Errorf("挂载模式的工作进程 UID %s 与数据目录 UID %d 不一致", value.WorkerUID, os.Getuid())
	}
	// The reservation prevents an unrelated /srv/resty directory being overwritten.
	if !value.BoundData {
		owner, e := c.externalExec(ctx, value, "/bin/sh", "-c", `if [ -d "$1" ]; then if [ -f "$1/.webcp-owner" ]; then cat "$1/.webcp-owner"; elif [ -n "$(ls -A "$1")" ]; then printf 'occupied'; fi; fi`, "--", Mount)
		if e != nil {
			return value, nil, e
		}
		if strings.TrimSpace(owner) != "" && strings.TrimSpace(owner) != c.owner() {
			return value, nil, fmt.Errorf("容器中的 %s 已被其他数据占用", Mount)
		}
	}
	return value, info, nil
}

func globalNginxDirectives(value string) (config *components.Config, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("1Panel 解析器无法解析原 Nginx 配置")
		}
	}()
	return parser.NewStringParser(value).Parse()
}
