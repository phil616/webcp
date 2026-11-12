package domain

import (
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"
)

// StreamProxy describes one L4 listener and one TCP/UDP destination. Host port
// publication belongs to the runtime; the container's internal port is not API state.
type StreamProxy struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Protocol       string    `json:"protocol"`
	ListenAddress  string    `json:"listenAddress"`
	ListenPort     int       `json:"listenPort"`
	TargetHost     string    `json:"targetHost"`
	TargetPort     int       `json:"targetPort"`
	ConnectTimeout int       `json:"connectTimeout"`
	ProxyTimeout   int       `json:"proxyTimeout"`
	UDPResponses   int       `json:"udpResponses"`
	ProxyProtocol  bool      `json:"proxyProtocol"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ListenerOverlap is deliberately conservative for IPv4/IPv6 wildcard binds.
// TCP and UDP namespaces are independent; callers compare protocols first.
func ListenerOverlap(addressA string, portA int, addressB string, portB int) bool {
	if portA != portB {
		return false
	}
	a, b := net.ParseIP(addressA), net.ParseIP(addressB)
	if a == nil || b == nil {
		return false
	}
	return a.IsUnspecified() || b.IsUnspecified() || a.Equal(b)
}

func ValidateStream(proxy StreamProxy) error {
	if strings.TrimSpace(proxy.Name) == "" || utf8.RuneCountInString(proxy.Name) > 128 {
		return fmt.Errorf("四层代理名称必须为 1–128 字符")
	}
	if proxy.Protocol != "tcp" && proxy.Protocol != "udp" {
		return fmt.Errorf("四层代理协议只能为 TCP 或 UDP")
	}
	if net.ParseIP(proxy.ListenAddress) == nil {
		return fmt.Errorf("监听地址必须是 IPv4 或 IPv6 地址")
	}
	if proxy.ListenPort < 1 || proxy.ListenPort > 65535 || proxy.TargetPort < 1 || proxy.TargetPort > 65535 {
		return fmt.Errorf("监听端口和目标端口必须为 1–65535")
	}
	if strings.TrimSpace(proxy.TargetHost) != proxy.TargetHost || proxy.TargetHost == "" || strings.HasPrefix(proxy.TargetHost, "*.") {
		return fmt.Errorf("目标必须是单个域名或 IP，不含协议和端口")
	}
	if err := ValidateDomains([]string{proxy.TargetHost}); err != nil {
		return fmt.Errorf("目标地址无效: %w", err)
	}
	if ip := net.ParseIP(proxy.TargetHost); ip != nil && (ip.IsUnspecified() || ip.IsMulticast()) {
		return fmt.Errorf("目标地址不能是通配地址或组播地址")
	}
	if proxy.ConnectTimeout < 1 || proxy.ConnectTimeout > 3600 || proxy.ProxyTimeout < 1 || proxy.ProxyTimeout > 86400 {
		return fmt.Errorf("连接超时需为 1–3600 秒，会话空闲超时需为 1–86400 秒")
	}
	if proxy.UDPResponses < 0 || proxy.UDPResponses > 1000 {
		return fmt.Errorf("UDP 响应报文数需为 0–1000")
	}
	if proxy.Protocol == "udp" && proxy.ProxyProtocol {
		return fmt.Errorf("UDP 不支持向后端发送 PROXY 协议")
	}
	return nil
}
