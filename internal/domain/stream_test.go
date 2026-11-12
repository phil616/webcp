package domain

import "testing"

func TestValidateStream(t *testing.T) {
	base := StreamProxy{Name: "代理", Protocol: "tcp", ListenAddress: "::1", ListenPort: 1234, TargetHost: "2001:db8::1", TargetPort: 443, ConnectTimeout: 10, ProxyTimeout: 600}
	if err := ValidateStream(base); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"*.example.test", "http://example.test", "example.test:443", "example.test;\nserver {}", "0.0.0.0", "::", "224.0.0.1", "bad host", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			p := base
			p.TargetHost = host
			if ValidateStream(p) == nil {
				t.Fatal("invalid target accepted")
			}
		})
	}
	p := base
	p.Protocol = "udp"
	p.ProxyProtocol = true
	if ValidateStream(p) == nil {
		t.Fatal("UDP PROXY protocol accepted")
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"0.0.0.0", "127.0.0.1", true}, {"::1", "0:0:0:0:0:0:0:1", true}, {"127.0.0.1", "127.0.0.2", false}, {"::", "127.0.0.1", true}} {
		if ListenerOverlap(tc.a, 42, tc.b, 42) != tc.want {
			t.Fatalf("overlap %+v", tc)
		}
	}
}
