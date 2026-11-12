package service

import (
	"net"
	"slices"
	"strings"
	"time"

	"web-panel/internal/domain"
)

func (m *Manager) SaveStream(revision uint64, proxy domain.StreamProxy) (domain.StreamProxy, error) {
	if proxy.ListenAddress == "" {
		proxy.ListenAddress = "0.0.0.0"
	}
	if ip := net.ParseIP(proxy.ListenAddress); ip != nil {
		proxy.ListenAddress = ip.String()
	}
	proxy.TargetHost = strings.ToLower(proxy.TargetHost)
	if ip := net.ParseIP(proxy.TargetHost); ip != nil {
		proxy.TargetHost = ip.String()
	}
	// Explicit defaults keep old API clients usable; stored snapshots are always complete.
	if proxy.ConnectTimeout == 0 {
		proxy.ConnectTimeout = 10
	}
	if proxy.ProxyTimeout == 0 {
		proxy.ProxyTimeout = 600
	}
	err := m.mutate(revision, func(state *domain.State) error {
		if proxy.ID == "" {
			proxy.ID = domain.NewID()
			proxy.CreatedAt = time.Now().UTC()
			state.Streams = append(state.Streams, proxy)
			return nil
		}
		i := slices.IndexFunc(state.Streams, func(item domain.StreamProxy) bool { return item.ID == proxy.ID })
		if i < 0 {
			return ErrNotFound
		}
		proxy.CreatedAt = state.Streams[i].CreatedAt
		state.Streams[i] = proxy
		return nil
	})
	return proxy, err
}

func (m *Manager) DeleteStream(revision uint64, id string) error {
	return m.mutate(revision, func(state *domain.State) error {
		i := slices.IndexFunc(state.Streams, func(item domain.StreamProxy) bool { return item.ID == id })
		if i < 0 {
			return ErrNotFound
		}
		state.Streams = slices.Delete(state.Streams, i, i+1)
		return nil
	})
}
