package openresty

import (
	"context"
	"path/filepath"
	"time"

	"web-panel/internal/domain"
)

// syncPageFiles mirrors only managed HTML names. Unlike a recursive docker cp,
// it also removes deleted overrides so inheritance works in copied containers.
// Each destination file is replaced atomically; unrelated assets are retained.
func (c *Controller) syncPageFiles(ctx context.Context, value Connection) error {
	stage := Mount + "/.page-sync-" + domain.NewID()
	if _, err := c.externalExec(ctx, value, "mkdir", "-p", stage); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = c.externalExec(cleanup, value, "rm", "-rf", "--", stage)
	}()
	if err := c.copyInto(ctx, value, filepath.Join(c.root, "runtime/root")+"/.", stage); err != nil {
		return err
	}
	if _, err := c.externalExec(ctx, value, "chown", "-R", value.WorkerUID+":"+value.WorkerGID, stage); err != nil {
		return err
	}
	const mirror = `set -eu
source=$1
target=$2
[ ! -L "$target" ] && [ ! -L "$target/sites" ]
mkdir -p "$target/sites"
sync_scope() {
  origin="$source/$1"
  destination="$target/$1"
  [ ! -L "$destination" ]
  mkdir -p "$destination"
  for code in 403 404 495 496 500 502 503 504; do
    if [ -f "$origin/$code.html" ]; then
      cp -p "$origin/$code.html" "$destination/.webcp-$code.tmp"
      mv -f "$destination/.webcp-$code.tmp" "$destination/$code.html"
    else
      rm -f "$destination/$code.html"
    fi
  done
}
sync_scope default
for directory in "$source/sites/"* "$target/sites/"*; do
  [ -d "$directory" ] || continue
  name=${directory##*/}
  [ "${#name}" -eq 32 ] || continue
  case "$name" in *[!0-9a-f]*) continue ;; esac
  sync_scope "sites/$name"
done
`
	_, err := c.externalExec(ctx, value, "/bin/sh", "-c", mirror, "--", stage, Mount+"/root")
	return err
}
