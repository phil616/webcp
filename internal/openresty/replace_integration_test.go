package openresty

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/store"
)

func TestReplacementRecoveryIntegration(t *testing.T) {
	if os.Getenv("RESTY_INTEGRATION") != "1" {
		t.Skip("set RESTY_INTEGRATION=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	port := func() string {
		t.Helper()
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		defer l.Close()
		return l.Addr().String()
	}
	d := &Docker{Root: s.Root, Container: "resty-recovery-" + domain.NewID()[:10], Image: "1panel/openresty:1.27.1.2-5-1-focal", HTTPBind: port(), HTTPSBind: port()}
	t.Cleanup(func() {
		out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=resty.manager.root="+s.Root).Output()
		for _, id := range strings.Fields(string(out)) {
			exec.Command("docker", "rm", "-f", id).Run()
		}
	})
	state := domain.Default()
	first, err := Render(s.Root, state)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Apply(ctx, first); err != nil {
		t.Fatal(err)
	}
	old, err := d.currentContainer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state.Revision++
	next, err := Render(s.Root, state)
	if err != nil {
		t.Fatal(err)
	}
	ports, err := d.bindings(next.Ports)
	if err != nil {
		t.Fatal(err)
	}
	id, err := d.createContainer(ctx, d.Container+"-candidate", ports)
	if err != nil {
		t.Fatal(err)
	}
	journal := replacementJournal{Container: d.Container, OldID: old.ID, NewID: id, OldTarget: "generations/" + first.ID, OldRunning: true}
	data, _ := json.Marshal(journal)
	if err = store.AtomicWrite(d.journalPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"stop", "--time", "1", old.ID}, {"rename", old.ID, d.Container + "-rollback"}} {
		if _, err = d.run(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = d.link("generations/" + next.ID); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"rename", id, d.Container}, {"start", id}} {
		if _, err = d.run(ctx, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a new process discovering a switch interrupted after the new engine started.
	restarted := *d
	if err = restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.currentContainer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != old.ID || !recovered.State.Running || restarted.Status(ctx).AppliedRevision != first.Revision {
		t.Fatal("old deployment was not recovered")
	}
	if _, err = os.Stat(d.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("journal retained: %v", err)
	}
	if candidate, e := d.existingByID(ctx, id); e != nil || candidate != nil {
		t.Fatalf("candidate retained: %v", e)
	}
	if err = restarted.Recover(ctx); err != nil {
		t.Fatal("recovery not idempotent:", err)
	}
}
