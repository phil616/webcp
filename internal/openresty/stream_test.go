package openresty

import (
	"slices"
	"strings"
	"testing"
	"web-panel/internal/domain"
)

func TestStreamRenderAndPublication(t *testing.T) {
	a := domain.StreamProxy{ID: domain.NewID(), Name: "TCP", Protocol: "tcp", ListenAddress: "127.0.0.1", ListenPort: 9000, TargetHost: "2001:db8::1", TargetPort: 443, ConnectTimeout: 10, ProxyTimeout: 600, Enabled: true, ProxyProtocol: true}
	b := a
	b.ID = domain.NewID()
	b.Name = "UDP"
	b.Protocol = "udp"
	b.ProxyProtocol = false
	b.TargetHost = "dns.example.test"
	b.TargetPort = 53
	b.UDPResponses = 1
	var first, second strings.Builder
	ports := renderStreams(&first, []domain.StreamProxy{a, b})
	other := renderStreams(&second, []domain.StreamProxy{b, a})
	if first.String() != second.String() || !slices.Equal(ports, other) {
		t.Fatal("unstable rendering")
	}
	for _, part := range []string{"stream {", " udp reuseport;", "proxy_pass [2001:db8::1]:443;", "proxy_pass dns.example.test:53;", "proxy_protocol on;", "proxy_responses 1;"} {
		if !strings.Contains(first.String(), part) {
			t.Fatalf("missing %s", part)
		}
	}
	d := &Docker{HTTPBind: "0.0.0.0:80", HTTPSBind: "0.0.0.0:443"}
	if err := d.ValidateStreams([]domain.StreamProxy{a, b}); err != nil {
		t.Fatal(err)
	}
	a.ListenPort = 80
	if d.ValidateStreams([]domain.StreamProxy{a}) == nil {
		t.Fatal("HTTP publication collision accepted")
	}
	a.Enabled = false
	if err := d.ValidateStreams([]domain.StreamProxy{a}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if len(renderStreams(&out, []domain.StreamProxy{a})) != 0 || out.Len() != 0 {
		t.Fatal("disabled listener rendered")
	}
}
