package openresty

import (
	"net"
	"slices"
	"strconv"
	"strings"
	"web-panel/internal/domain"
)

// WebsitePort is a logical website port; the runtime resolves CLI host bindings.
type WebsitePort struct {
	Port, ContainerPort int
	SSL, UDP            bool
}

func websitePorts(state domain.State) (map[int]int, []WebsitePort, error) {
	listens, err := domain.WebsiteListeners(state)
	if err != nil {
		return nil, nil, err
	}
	keys := []int{}
	for port := range listens {
		if port != 80 && port != 443 {
			keys = append(keys, port)
		}
	}
	slices.Sort(keys)
	internal := map[int]int{80: 80, 443: 443}
	ports := []WebsitePort{}
	for i, port := range keys {
		internal[port] = 20000 + i
		ports = append(ports, WebsitePort{Port: port, ContainerPort: 20000 + i, SSL: listens[port]})
	}
	quic := map[int]bool{}
	for _, raw := range state.Sites {
		s := domain.SiteDefaults(raw)
		if !s.Enabled || s.TLSID == "" || !s.HTTPS.HTTP3 {
			continue
		}
		quic[443] = true
		for _, b := range s.Bindings {
			if b.SSL {
				quic[b.Port] = true
			}
		}
	}
	keys = []int{}
	for port := range quic {
		keys = append(keys, port)
	}
	slices.Sort(keys)
	for _, port := range keys {
		ports = append(ports, WebsitePort{Port: port, ContainerPort: internal[port], SSL: true, UDP: true})
	}
	return internal, ports, nil
}
func (d *Docker) ValidateState(state domain.State) error {
	_, web, err := websitePorts(state)
	if err != nil {
		return err
	}
	var out strings.Builder
	_, err = d.bindings(renderStreams(&out, state.Streams), web...)
	return err
}

// websiteBindings resolves custom ports against the same host IP as website listeners.
func (d *Docker) websiteBindings(web []WebsitePort) ([]PortBinding, error) {
	ports := []PortBinding{}
	for _, p := range web {
		bind := d.HTTPBind
		if p.SSL {
			bind = d.HTTPSBind
		}
		host, configured, err := net.SplitHostPort(bind)
		if err != nil {
			return nil, err
		}
		number := p.Port
		if p.Port == 443 {
			number, err = strconv.Atoi(configured)
			if err != nil {
				return nil, err
			}
		}
		protocol := "tcp"
		if p.UDP {
			protocol = "udp"
		}
		ports = append(ports, PortBinding{HostAddress: host, HostPort: number, ContainerPort: p.ContainerPort, Protocol: protocol})
	}
	return ports, nil
}

func (d *Docker) RenderOptions() RenderOptions {
	_, port, _ := net.SplitHostPort(d.HTTPSBind)
	number, _ := strconv.Atoi(port)
	return RenderOptions{HTTPSPort: number}
}
