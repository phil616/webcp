package service

import (
	"context"
	"errors"
	"testing"
	"web-panel/internal/domain"
)

func TestOpenRestyPerformanceSaveApplyAndConflict(t *testing.T) {
	m, store, runtime := setup(t)
	ctx := context.Background()
	rev := revisionOf(t, store)
	if err := m.SaveOpenRestyPerformance(ctx, rev, map[string]string{"gzip": "on", "keepalive_timeout": "1m30s 90"}); err != nil {
		t.Fatal(err)
	}
	if runtime.applied != rev+1 {
		t.Fatal("running runtime was not reloaded")
	}
	if err := m.SaveOpenRestyPerformance(ctx, rev, map[string]string{"gzip": "off"}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	if err := m.SaveOpenRestyPerformance(ctx, rev+1, map[string]string{"gzip": "on; invalid;"}); err == nil {
		t.Fatal("directive injection accepted")
	}
	if revisionOf(t, store) != rev+1 {
		t.Fatal("invalid update changed state")
	}
	runtime.fail = true
	if err := m.SaveOpenRestyPerformance(ctx, rev+1, map[string]string{"gzip_comp_level": "3"}); err == nil {
		t.Fatal("deployment failure hidden")
	}
	if runtime.applied != rev+1 {
		t.Fatal("failed application replaced live generation")
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if domain.EffectiveOpenResty(state.OpenResty).HTTP["gzip_comp_level"] != "3" {
		t.Fatal("saved draft lost after apply failure")
	}
	runtime.fail = false
	runtime.stopped = true
	if err = m.SaveOpenRestyPerformance(ctx, state.Revision, map[string]string{"gzip": "off"}); err != nil {
		t.Fatal(err)
	}
	if runtime.applied != rev+1 {
		t.Fatal("saving a stopped service started it")
	}
}
