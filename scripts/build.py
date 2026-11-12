#!/usr/bin/env python3
"""Build the embedded WebCP executable with explicit, reproducible identity."""

import argparse
import datetime
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent


def version():
    value = (ROOT / "VERSION").read_text().strip()
    if not re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?", value
    ):
        raise SystemExit("VERSION must contain a semantic release version")
    return value


def identity():
    commit = os.environ.get("WEBCP_BUILD_COMMIT", "")
    if not commit:
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True
        )
        commit = result.stdout.strip() if result.returncode == 0 else "unknown"
    if commit != "unknown" and not re.fullmatch(r"[0-9a-f]{40,64}", commit):
        raise SystemExit("WEBCP_BUILD_COMMIT must be a full commit hash or unknown")
    epoch = os.environ.get("SOURCE_DATE_EPOCH")
    build_time = (
        datetime.datetime.fromtimestamp(int(epoch), datetime.timezone.utc)
        .isoformat()
        .replace("+00:00", "Z")
        if epoch
        else "unknown"
    )
    return version(), commit, build_time


def build(output, arch):
    if arch not in ("amd64", "arm64"):
        raise SystemExit("Supported release architectures: amd64, arm64")
    if not (ROOT / "web/dist/index.html").is_file():
        raise SystemExit("Build the frontend first: make web-build")
    release, commit, built = identity()
    flags = "-s -w " + " ".join(
        f"-X web-panel/internal/buildinfo.{key}={value}"
        for key, value in (
            ("Version", release),
            ("Commit", commit),
            ("BuildTime", built),
        )
    )
    output = Path(output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    environment = dict(os.environ, GOOS="linux", GOARCH=arch, CGO_ENABLED="0")
    subprocess.run(
        [
            "go",
            "build",
            "-trimpath",
            "-buildvcs=false",
            "-ldflags",
            flags,
            "-o",
            str(output),
            "./cmd/web-panel",
        ],
        cwd=ROOT,
        env=environment,
        check=True,
    )
    output.chmod(0o755)
    print(f"Built WebCP {release} for linux/{arch}: {output}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "bin/webcp")
    parser.add_argument("--arch", choices=("amd64", "arm64"), default="amd64")
    args = parser.parse_args()
    build(args.output, args.arch)
