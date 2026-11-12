package openresty

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"web-panel/internal/domain"
)

// PortBinding separates host publication from the stable, private container listener.
type PortBinding struct {
	HostAddress   string `json:"hostAddress"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

func renderStreams(out *strings.Builder, streams []domain.StreamProxy) []PortBinding {
	active := []domain.StreamProxy{}
	for _, proxy := range streams {
		if proxy.Enabled {
			active = append(active, proxy)
		}
	}
	slices.SortFunc(active, func(a, b domain.StreamProxy) int { return strings.Compare(a.ID, b.ID) })
	ports := make([]PortBinding, 0, len(active))
	if len(active) == 0 {
		return ports
	}
	out.WriteString("stream {\n log_format stream_log '$remote_addr [$time_local] $protocol $status $bytes_sent $bytes_received $session_time';\n access_log /dev/stdout stream_log;\n")
	for i, proxy := range active {
		// Allocation never overlaps the HTTP/HTTPS container listeners (80/443).
		port := 10000 + i
		ports = append(ports, PortBinding{HostAddress: proxy.ListenAddress, HostPort: proxy.ListenPort, ContainerPort: port, Protocol: proxy.Protocol})
		fmt.Fprintf(out, " # stream %s\n server {\n  listen %d", proxy.ID, port)
		if proxy.Protocol == "udp" {
			out.WriteString(" udp reuseport")
		}
		fmt.Fprintf(out, ";\n  proxy_pass %s;\n  proxy_connect_timeout %ds;\n  proxy_timeout %ds;\n", net.JoinHostPort(proxy.TargetHost, strconv.Itoa(proxy.TargetPort)), proxy.ConnectTimeout, proxy.ProxyTimeout)
		if proxy.Protocol == "udp" {
			fmt.Fprintf(out, "  proxy_responses %d;\n", proxy.UDPResponses)
		} else {
			out.WriteString("  proxy_half_close on;\n  proxy_socket_keepalive on;\n")
		}
		if proxy.ProxyProtocol {
			out.WriteString("  proxy_protocol on;\n")
		}
		out.WriteString(" }\n")
	}
	out.WriteString("}\n")
	return ports
}
