#!/usr/bin/env python3
"""Package Linux binaries and source; use make release for the full validation."""

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile

from build import ROOT, build, identity

SOURCE_ROOTS = ("cmd", "internal", "web", "scripts", "docs", ".github")
SOURCE_FILES = (
    "VERSION",
    "README.md",
    "CHANGELOG.md",
    "CONTRIBUTING.md",
    "LICENSE",
    "NOTICE",
    "Makefile",
    ".gitignore",
    ".prettierrc.json",
    "go.mod",
    "go.sum",
    "webcp.example.json",
)
EXCLUDED = {"node_modules", "__pycache__", "test-results", "playwright-report", "dist"}


def source_files():
    result = [ROOT / name for name in SOURCE_FILES]
    for directory in SOURCE_ROOTS:
        for path in (ROOT / directory).rglob("*"):
            relative = path.relative_to(ROOT)
            if not path.is_file() or any(part in EXCLUDED for part in relative.parts):
                continue
            if path.suffix in (".log", ".pyc"):
                continue
            if path.is_symlink():
                raise SystemExit(f"Release source cannot contain symlinks: {relative}")
            result.append(path)
    return sorted(set(result))


def archive(output, prefix, entries):
    epoch = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))
    with (
        output.open("wb") as destination,
        gzip.GzipFile(
            filename="", fileobj=destination, mode="wb", mtime=epoch
        ) as compressed,
        tarfile.open(
            fileobj=compressed, mode="w|", format=tarfile.PAX_FORMAT
        ) as writer,
    ):
        for name, path in sorted(entries):
            info = tarfile.TarInfo(prefix + "/" + name)
            data = path.read_bytes()
            info.size, info.mtime = len(data), epoch
            info.mode = (
                0o755 if name == "webcp" or path.stat().st_mode & 0o111 else 0o644
            )
            writer.addfile(info, io.BytesIO(data))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/releases")
    parser.add_argument("--arch", action="append", choices=("amd64", "arm64"))
    args = parser.parse_args()
    release, commit, built = identity()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    names = [
        f"webcp-{release}-linux-{arch}.tar.gz"
        for arch in (args.arch or ["amd64", "arm64"])
    ]
    names += [f"webcp-{release}-source.tar.gz", "SHA256SUMS", "release.json"]
    if any((output / name).exists() for name in names):
        raise SystemExit(
            "Refusing to replace existing release artifacts; select a fresh --output directory"
        )
    subprocess.run(
        ["python3", "scripts/dependency-notices.py", "--check"], cwd=ROOT, check=True
    )
    artifacts = []
    with tempfile.TemporaryDirectory(prefix="webcp-release-") as directory:
        for arch in args.arch or ["amd64", "arm64"]:
            executable = Path(directory) / arch / "webcp"
            build(executable, arch)
            destination = output / f"webcp-{release}-linux-{arch}.tar.gz"
            entries = [
                ("webcp", executable),
                ("README.md", ROOT / "docs/installation.md"),
            ]
            entries += [
                (name, ROOT / name)
                for name in ("LICENSE", "NOTICE", "webcp.example.json")
            ]
            entries += [
                (path.name, path)
                for path in (ROOT / "web/public/licenses").iterdir()
                if path.is_file()
            ]
            archive(destination, f"webcp-{release}-linux-{arch}", entries)
            artifacts.append(destination)
    source = output / f"webcp-{release}-source.tar.gz"
    archive(
        source,
        f"webcp-{release}-source",
        [(str(path.relative_to(ROOT)), path) for path in source_files()],
    )
    artifacts.append(source)
    metadata = {
        "version": release,
        "commit": commit,
        "buildTime": built,
        "goVersion": subprocess.check_output(
            ["go", "env", "GOVERSION"], cwd=ROOT, text=True
        ).strip(),
        "artifacts": [
            {
                "file": path.name,
                "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                "size": path.stat().st_size,
            }
            for path in artifacts
        ],
    }
    manifest = output / "release.json"
    manifest.write_text(json.dumps(metadata, indent=2) + "\n")
    artifacts.append(manifest)
    (output / "SHA256SUMS").write_text(
        "".join(
            hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name + "\n"
            for path in artifacts
        )
    )
    print(f"Packaged WebCP {release}: {output}")


if __name__ == "__main__":
    main()
