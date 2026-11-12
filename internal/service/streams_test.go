package service

import (
	"context"
	"errors"
	"testing"
	"web-panel/internal/domain"
)

func TestStreamCRUDAndBackup(t *testing.T) {
	m, s, _ := setup(t)
	p, err := m.SaveStream(revisionOf(t, s), domain.StreamProxy{Name: "数据库", Protocol: "tcp", ListenPort: 15432, TargetHost: "DB.Example.test", TargetPort: 5432, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ListenAddress != "0.0.0.0" || p.TargetHost != "db.example.test" || p.ConnectTimeout != 10 {
		t.Fatalf("defaults: %+v", p)
	}
	if _, err = m.SaveStream(1, p); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	duplicate := p
	duplicate.ID = ""
	duplicate.Name = "重复"
	if _, err = m.SaveStream(revisionOf(t, s), duplicate); err == nil {
		t.Fatal("overlapping TCP listener accepted")
	}
	duplicate.Protocol = "udp"
	if _, err = m.SaveStream(revisionOf(t, s), duplicate); err != nil {
		t.Fatal(err)
	}
	record, err := m.Backup(context.Background(), revisionOf(t, s), false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteStream(revisionOf(t, s), p.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Restore(context.Background(), revisionOf(t, s), record.ID, false); err != nil {
		t.Fatal(err)
	}
	state, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Streams) != 2 || state.Streams[0].ID != p.ID {
		t.Fatal("backup lost streams")
	}
}
