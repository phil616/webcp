package service

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"web-panel/internal/domain"
)

type NginxParam struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
}

func (m *Manager) OpenRestyPerformance() ([]NginxParam, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	config := domain.EffectiveOpenResty(state.OpenResty)
	result := []NginxParam{}
	for _, name := range domain.OpenRestyDirectives(&config) {
		result = append(result, NginxParam{Name: name, Params: strings.Fields(config.HTTP[name])})
	}
	return result, nil
}
func (m *Manager) SaveOpenRestyPerformance(ctx context.Context, rev uint64, params map[string]string) error {
	if len(params) == 0 {
		return fmt.Errorf("OpenResty 参数不能为空")
	}
	err := m.mutate(rev, func(state *domain.State) error {
		config := domain.EffectiveOpenResty(state.OpenResty)
		maps.Copy(config.HTTP, params)
		state.OpenResty = &config
		return domain.ValidateOpenResty(&config)
	})
	if err != nil {
		return err
	}
	if m.runtime.Status(ctx).Running {
		if err = m.Apply(ctx, rev+1); err != nil {
			return fmt.Errorf("配置已保存，但校验并应用失败：%w", err)
		}
	}
	return nil
}
