package service

import (
	"context"
	"strings"

	"github.com/go-acme/lego/v5/challenge/http01"
)

func (m *Manager) syncRuntimeFile(ctx context.Context, kind, site, name string, remove bool) error {
	if runtime, ok := m.runtime.(interface {
		SyncFile(context.Context, string, string, string, bool) error
	}); ok {
		return runtime.SyncFile(ctx, kind, site, name, remove)
	}
	return nil
}

type syncedHTTPChallenge struct {
	provider interface {
		Present(context.Context, string, string, string) error
		CleanUp(context.Context, string, string, string) error
	}
	sync func(context.Context, string, bool) error
}

func (p *syncedHTTPChallenge) Present(ctx context.Context, domain, token, key string) error {
	if err := p.provider.Present(ctx, domain, token, key); err != nil {
		return err
	}
	return p.sync(ctx, strings.TrimPrefix(http01.ChallengePath(token), "/"), false)
}
func (p *syncedHTTPChallenge) CleanUp(ctx context.Context, domain, token, key string) error {
	if err := p.provider.CleanUp(ctx, domain, token, key); err != nil {
		return err
	}
	return p.sync(ctx, strings.TrimPrefix(http01.ChallengePath(token), "/"), true)
}
