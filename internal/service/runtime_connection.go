package service

import (
	"context"
	"errors"
	"web-panel/internal/openresty"
)

type runtimeConnections interface {
	Connection() openresty.Connection
	Configure(context.Context, uint64, openresty.Connection) error
	Probe(context.Context, openresty.Connection) (openresty.Probe, error)
	Containers(context.Context) ([]openresty.ContainerChoice, error)
}

func (m *Manager) RuntimeConnection() (openresty.Connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.runtime.(runtimeConnections)
	if !ok {
		return openresty.Connection{}, ErrNotFound
	}
	return runtime.Connection(), nil
}
func (m *Manager) ConfigureRuntime(ctx context.Context, version uint64, value openresty.Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.runtime.(runtimeConnections)
	if !ok {
		return ErrNotFound
	}
	err := runtime.Configure(ctx, version, value)
	if errors.Is(err, openresty.ErrConnectionConflict) {
		return ErrConflict
	}
	return err
}
func (m *Manager) ProbeRuntime(ctx context.Context, value openresty.Connection) (openresty.Probe, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.runtime.(runtimeConnections)
	if !ok {
		return openresty.Probe{}, ErrNotFound
	}
	return runtime.Probe(ctx, value)
}
func (m *Manager) RuntimeContainers(ctx context.Context) ([]openresty.ContainerChoice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime, ok := m.runtime.(runtimeConnections)
	if !ok {
		return nil, ErrNotFound
	}
	return runtime.Containers(ctx)
}
