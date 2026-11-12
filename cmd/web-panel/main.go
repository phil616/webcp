package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"web-panel/internal/buildinfo"
	"web-panel/internal/config"
	"web-panel/internal/httpapi"
	"web-panel/internal/openresty"
	"web-panel/internal/service"
	"web-panel/internal/store"
	"web-panel/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	showVersion := flag.Bool("version", false, "打印构建版本后退出")
	configFile := flag.String("config", "", "WebCP JSON 配置文件")
	listen := flag.String("listen", "127.0.0.1:9080", "管理界面监听地址")
	data := flag.String("data", "./data", "持久化数据目录")
	image := flag.String("image", "1panel/openresty:1.27.1.2-5-1-focal", "OpenResty 容器镜像")
	container := flag.String("container", "webcp-openresty", "OpenResty 容器名称")
	httpBind := flag.String("http-bind", "0.0.0.0:80", "网站 HTTP 主机地址与端口")
	httpsBind := flag.String("https-bind", "0.0.0.0:443", "网站 HTTPS 主机地址与端口")
	flag.Parse()
	if *showVersion {
		return json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
	}
	settings, err := config.Load(*configFile)
	if err != nil {
		return err
	}
	supplied := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { supplied[f.Name] = true })
	for _, item := range []struct {
		name   string
		target *string
		value  string
	}{{"listen", listen, settings.Listen}, {"data", data, settings.Data}, {"container", container, settings.Container}, {"image", image, settings.Image}, {"http-bind", httpBind, settings.HTTPBind}, {"https-bind", httpsBind, settings.HTTPSBind}} {
		if !supplied[item.name] && item.value != "" {
			*item.target = item.value
		}
	}

	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(*container) {
		return fmt.Errorf("容器名称无效")
	}
	dataRoot, err := filepath.Abs(*data)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dataRoot, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dataRoot, "manager.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("数据目录已被另一个管理器使用: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	s, err := store.Open(dataRoot)
	if err != nil {
		return err
	}
	defer s.Close()
	runtime, err := openresty.NewController(s.Root, openresty.Connection{Mode: "managed", Image: *image, Container: *container, HTTPBind: *httpBind, HTTPSBind: *httpsBind}, s)
	if err != nil {
		return err
	}
	manager, err := service.New(s, runtime)
	if err != nil {
		return err
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return err
	}
	server := httpapi.Server(*listen, httpapi.New(manager, assets, settings.AccessKey, settings.SecureCookie))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	manager.StartSSLWorker(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "error", err)
		}
	}()
	slog.Info("WebCP listening", "address", *listen, "data", s.Root, "container", *container)
	if err = server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
