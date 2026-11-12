// Package config loads startup settings; secrets are never part of website backups.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxConfigSize = 64 << 10

type Config struct {
	AccessKey    string `json:"accessKey"`
	SecureCookie bool   `json:"secureCookie"`
	Listen       string `json:"listen"`
	Data         string `json:"data"`
	Container    string `json:"container"`
	Image        string `json:"image"`
	HTTPBind     string `json:"httpBind"`
	HTTPSBind    string `json:"httpsBind"`
}

func Load(path string) (Config, error) {
	var config Config
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return config, fmt.Errorf("读取 WebCP 配置失败: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
		if err != nil {
			return config, fmt.Errorf("读取 WebCP 配置失败: %w", err)
		}
		if len(data) > maxConfigSize {
			return config, fmt.Errorf("WebCP 配置文件超过 64 KiB 限制")
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 || data[0] != '{' {
			return config, fmt.Errorf("WebCP 配置必须为单个 JSON 对象")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&config); err != nil {
			return config, fmt.Errorf("WebCP 配置 JSON 无效: %w", err)
		}
		if err = decoder.Decode(new(any)); err != io.EOF {
			return config, fmt.Errorf("WebCP 配置必须为单个 JSON 对象")
		}
	}
	key, keySet := os.LookupEnv("WEBCP_ACCESS_KEY")
	keyFile := os.Getenv("WEBCP_ACCESS_KEY_FILE")
	if keySet && keyFile != "" {
		return config, fmt.Errorf("WEBCP_ACCESS_KEY 与 WEBCP_ACCESS_KEY_FILE 不能同时设置")
	}
	if keyFile != "" {
		file, err := os.Open(keyFile)
		if err != nil {
			return config, fmt.Errorf("读取访问密钥文件失败: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxConfigSize+1))
		if err != nil {
			return config, fmt.Errorf("读取访问密钥文件失败: %w", err)
		}
		if len(data) > maxConfigSize {
			return config, fmt.Errorf("访问密钥文件超过 64 KiB 限制")
		}
		key = strings.TrimRight(string(data), "\r\n")
		keySet = true
	}
	if keySet {
		config.AccessKey = key
	}
	if len(config.AccessKey) < 16 || len(config.AccessKey) > 4096 || strings.TrimSpace(config.AccessKey) != config.AccessKey {
		return config, fmt.Errorf("请通过配置文件 accessKey、WEBCP_ACCESS_KEY 或 WEBCP_ACCESS_KEY_FILE 注入 16–4096 字节访问密钥")
	}
	return config, nil
}
