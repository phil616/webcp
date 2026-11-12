package openresty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"web-panel/internal/store"
)

func TestRuntimeConnectionPersistsAndRejectsStaleChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	repository, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	initial := Connection{Container: "webcp-test", Image: "custom/openresty:local", HTTPBind: "127.0.0.1:18080", HTTPSBind: "127.0.0.1:18443"}
	controller, err := NewController(root, initial, repository)
	if err != nil {
		t.Fatal(err)
	}
	value := controller.Connection()
	value.Container = "other-openresty"
	if err = controller.Configure(context.Background(), 1, value); err != nil {
		t.Fatal(err)
	}
	if err = controller.Configure(context.Background(), 1, value); !errors.Is(err, ErrConnectionConflict) {
		t.Fatal("stale connection update accepted", err)
	}
	loaded, err := NewController(root, initial, repository)
	if err != nil || loaded.Connection().Container != "other-openresty" {
		t.Fatal("connection lost", err)
	}
	value.Container = "--invalid"
	if err = controller.Configure(context.Background(), 2, value); err == nil {
		t.Fatal("invalid container accepted")
	}
}
