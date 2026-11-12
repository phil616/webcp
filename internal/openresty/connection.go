package openresty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"web-panel/internal/domain"
	"web-panel/internal/store"
)

// Connection is machine-local runtime configuration, separate from portable website backups.
var ErrConnectionConflict = errors.New("容器连接配置已变化，请刷新后重试")

type Connection struct {
	ContainerID string   `json:"containerId,omitempty"`
	Version     uint64   `json:"version"`
	Mode        string   `json:"mode"`
	Container   string   `json:"container"`
	Image       string   `json:"image"`
	HTTPBind    string   `json:"httpBind"`
	HTTPSBind   string   `json:"httpsBind"`
	Binary      string   `json:"binary,omitempty"`
	LuaJIT      string   `json:"luaJit,omitempty"`
	MainConfig  string   `json:"mainConfig,omitempty"`
	WorkerUser  string   `json:"workerUser,omitempty"`
	WorkerGroup string   `json:"workerGroup,omitempty"`
	WorkerUID   string   `json:"workerUid,omitempty"`
	WorkerGID   string   `json:"workerGid,omitempty"`
	MasterUID   string   `json:"masterUid,omitempty"`
	MIMEPath    string   `json:"mimePath,omitempty"`
	LoadModules []string `json:"loadModules,omitempty"`
	BoundData   bool     `json:"boundData"`
}
type ContainerChoice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
}
type Probe struct {
	Compatible bool        `json:"compatible"`
	Problems   []string    `json:"problems"`
	Status     Status      `json:"status"`
	Connection *Connection `json:"connection,omitempty"`
}

// Controller preserves the existing managed Docker runtime and adds explicit,
// persistent runtime selection. It never discovers or changes another container implicitly.
type Controller struct {
	mu         sync.RWMutex
	op         sync.Mutex
	root       string
	connection Connection
	repository *store.Store
}

func NewController(root string, initial Connection, repository *store.Store) (*Controller, error) {
	c := &Controller{root: root, connection: initial, repository: repository}
	if c.connection.Mode == "" {
		c.connection.Mode = "managed"
	}
	if c.connection.Version == 0 {
		c.connection.Version = 1
	}
	data, err := repository.LoadConnection()
	if err == nil && len(data) > 0 {
		if err = json.Unmarshal(data, &c.connection); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err = validateConnection(c.connection); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Controller) Connection() Connection { c.mu.RLock(); defer c.mu.RUnlock(); return c.connection }
func (c *Controller) docker(value Connection) *Docker {
	return &Docker{Root: c.root, Container: value.Container, Image: value.Image, HTTPBind: value.HTTPBind, HTTPSBind: value.HTTPSBind}
}
func validateConnection(value Connection) error {
	if value.ContainerID != "" && !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value.ContainerID) {
		return fmt.Errorf("无效容器 ID")
	}
	if value.Mode != "managed" && value.Mode != "existing" {
		return fmt.Errorf("容器模式必须为 managed 或 existing")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(value.Container) {
		return fmt.Errorf("无效容器名称")
	}
	if value.Image == "" || strings.HasPrefix(value.Image, "-") || strings.ContainsAny(value.Image, " \r\n\t") {
		return fmt.Errorf("请填写有效镜像")
	}
	for _, binary := range []string{value.Binary, value.LuaJIT} {
		if binary != "" && !safeExecutable(binary) {
			return fmt.Errorf("无效可执行程序路径")
		}
	}
	for _, location := range append([]string{value.MainConfig, value.MIMEPath}, value.LoadModules...) {
		if location != "" && !safeContainerPath(location) {
			return fmt.Errorf("无效容器路径")
		}
	}
	for _, name := range []string{value.WorkerUser, value.WorkerGroup} {
		if name != "" && !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(name) {
			return fmt.Errorf("无效工作进程用户或组")
		}
	}
	for _, id := range []string{value.WorkerUID, value.WorkerGID, value.MasterUID} {
		if id != "" && !regexp.MustCompile(`^[0-9]{1,10}$`).MatchString(id) {
			return fmt.Errorf("无效工作进程 UID/GID")
		}
	}
	return (&Docker{HTTPBind: value.HTTPBind, HTTPSBind: value.HTTPSBind}).ValidateStreams(nil)
}
func (c *Controller) Containers(ctx context.Context) ([]ContainerChoice, error) {
	output, err := c.docker(c.Connection()).run(ctx, "container", "ls", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	result := []ContainerChoice{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var item struct{ ID, Names, Image, Status string }
		if err = json.Unmarshal([]byte(line), &item); err != nil {
			return nil, err
		}
		result = append(result, ContainerChoice{item.ID, item.Names, item.Image, item.Status})
	}
	return result, nil
}
func inspectContainer(ctx context.Context, d *Docker) (*containerInfo, error) {
	output, err := d.run(ctx, "inspect", "--format", "{{json .}}", d.Container)
	if err != nil {
		return nil, err
	}
	var info containerInfo
	err = json.Unmarshal([]byte(output), &info)
	return &info, err
}
func (c *Controller) Probe(ctx context.Context, value Connection) (Probe, error) {
	result := Probe{Problems: []string{}}
	if err := validateConnection(value); err != nil {
		return result, err
	}
	d := c.docker(value)
	if value.Mode == "managed" {
		result.Status = d.Status(ctx)
		result.Compatible = result.Status.Available
		if !result.Status.Available {
			result.Problems = append(result.Problems, result.Status.Detail)
		} else if info, e := inspectContainer(ctx, d); e == nil && info.Config.Labels["resty.manager.root"] != c.root {
			result.Compatible = false
			result.Problems = append(result.Problems, "该名称已被其他容器使用，请选择连接已有容器或更换名称")
		}
		return result, nil
	}
	detected, info, err := c.discoverExternal(ctx, value)
	if err != nil {
		return result, err
	}
	result.Status = externalStatus(d, info)
	result.Compatible = true
	result.Connection = &detected
	return result, nil
}

func (c *Controller) Configure(ctx context.Context, expected uint64, value Connection) error {
	c.op.Lock()
	defer c.op.Unlock()
	if err := validateConnection(value); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection.Version != expected {
		return ErrConnectionConflict
	}
	if value.Mode == "existing" {
		probe, err := c.Probe(ctx, value)
		if err != nil {
			return err
		}
		if !probe.Compatible {
			return fmt.Errorf("容器尚不兼容：%s", strings.Join(probe.Problems, "；"))
		}
		value = *probe.Connection
	}
	if value.Mode == "existing" {
		info, err := c.selectedExisting(ctx, value)
		if err != nil {
			return err
		}
		value.ContainerID = info.ID
	} else {
		value.ContainerID = ""
	}
	value.Version = expected + 1
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = c.repository.SaveConnection(expected, data); err != nil {
		return err
	}
	c.connection = value
	return nil
}
func externalStatus(d *Docker, info *containerInfo) Status {
	result := Status{Available: true, Running: info.State.Running, Container: d.Container, Image: info.Config.Image, PublishedPorts: []PortBinding{}}
	result.PublishedPorts, _ = publishedBindings(info)
	if data, err := os.ReadFile(filepath.Join(d.Root, "runtime/current/revision")); err == nil {
		fmt.Sscan(string(data), &result.AppliedRevision)
	}
	return result
}
func (c *Controller) Status(ctx context.Context) Status {
	value := c.Connection()
	d := c.docker(value)
	if value.Mode == "managed" {
		return d.Status(ctx)
	}
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return Status{Container: value.Container, Image: value.Image, Detail: err.Error(), PublishedPorts: []PortBinding{}}
	}
	status := externalStatus(d, info)
	if value.MainConfig != Mount+"/current/nginx.conf" {
		status.AppliedRevision = 0
		if data, err := os.ReadFile(filepath.Join(c.externalDir(value), "revision")); err == nil {
			fmt.Sscan(string(data), &status.AppliedRevision)
		}
	}
	return status
}
func (c *Controller) Apply(ctx context.Context, g Generation) error {
	c.op.Lock()
	defer c.op.Unlock()
	value := c.Connection()
	d := c.docker(value)
	if value.Mode == "managed" {
		return d.Apply(ctx, g)
	}
	return c.applyTakeover(ctx, value, g)
}
func (c *Controller) Stop(ctx context.Context) error {
	c.op.Lock()
	defer c.op.Unlock()
	value := c.Connection()
	d := c.docker(value)
	if value.Mode == "managed" {
		return d.Stop(ctx)
	}
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return err
	}
	_, err = d.run(ctx, "stop", "--time", "10", info.ID)
	return err
}
func (c *Controller) Logs(ctx context.Context) (string, error) {
	value := c.Connection()
	if value.Mode == "managed" {
		return c.docker(value).Logs(ctx)
	}
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return "", err
	}
	return c.docker(value).run(ctx, "logs", "--tail", "200", info.ID)
}
func (c *Controller) ValidateState(state domain.State) error {
	return c.docker(c.Connection()).ValidateState(state)
}
func (c *Controller) RenderOptions() RenderOptions {
	value := c.Connection()
	options := c.docker(value).RenderOptions()
	if value.Mode == "existing" {
		user := ""
		if value.MasterUID == "0" {
			user = value.WorkerUser
			if value.WorkerGroup != "" {
				user += " " + value.WorkerGroup
			}
		}
		options.WorkerUser = &user
		options.MIMEPath = value.MIMEPath
		options.CopySites = !value.BoundData
		options.LoadModules = value.LoadModules
	}
	return options
}
func (c *Controller) Recover(ctx context.Context) error {
	value := c.Connection()
	if value.Mode == "managed" {
		return c.docker(value).Recover(ctx)
	}
	return c.recoverTakeover(ctx, value)
}
func (c *Controller) ClearProxyCache(ctx context.Context, id string) error {
	value := c.Connection()
	if value.Mode == "managed" {
		return c.docker(value).ClearProxyCache(ctx, id)
	}
	if !domain.ValidID(id) {
		return fmt.Errorf("无效网站 ID")
	}
	info, err := c.selectedExisting(ctx, value)
	if err != nil {
		return err
	}
	_, err = c.docker(value).run(ctx, "exec", info.ID, "find", Mount+"/cache/"+id, "-type", "f", "-delete")
	return err
}

func (c *Controller) selectedExisting(ctx context.Context, value Connection) (*containerInfo, error) {
	d := c.docker(value)
	if value.ContainerID != "" {
		d.Container = value.ContainerID
	}
	return inspectContainer(ctx, d)
}
