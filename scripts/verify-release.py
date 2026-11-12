#!/usr/bin/env python3
"""Verify delivered bytes, archive boundaries and an optional corresponding-source rebuild."""

import argparse
import datetime
import hashlib
import json
import os
import platform
from pathlib import Path, PurePosixPath
import subprocess
import tarfile
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verify(directory, rebuild, private_config):
    metadata = json.loads((directory / "release.json").read_text())
    version = metadata["version"]
    sums = {}
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        checksum, name = line.split("  ", 1)
        if Path(name).name != name or name in sums or len(checksum) != 64:
            raise SystemExit("Invalid checksum manifest")
        if digest(directory / name) != checksum:
            raise SystemExit(f"Checksum mismatch: {name}")
        sums[name] = checksum
    expected = {item["file"] for item in metadata["artifacts"]} | {"release.json"}
    if set(sums) != expected:
        raise SystemExit("Checksum and release manifests disagree")
    for item in metadata["artifacts"]:
        path = directory / item["file"]
        if digest(path) != item["sha256"] or path.stat().st_size != item["size"]:
            raise SystemExit("Release metadata does not match delivered bytes")
    secret = None
    if private_config:
        secret = json.loads(private_config.read_text())["accessKey"].encode()
        if not secret:
            raise SystemExit("Private configuration contains no key to verify")
    host_arch = (
        {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine())
        if platform.system() == "Linux"
        else None
    )
    native = None
    source_path = None
    with tempfile.TemporaryDirectory(prefix="webcp-delivery-verify-") as temporary:
        temporary = Path(temporary)
        for item in metadata["artifacts"]:
            path = directory / item["file"]
            prefix = path.name.removesuffix(".tar.gz")
            with tarfile.open(path, "r:gz") as archive:
                names = set()
                for member in archive.getmembers():
                    relative = PurePosixPath(member.name)
                    if (
                        not member.isfile()
                        or relative.is_absolute()
                        or ".." in relative.parts
                        or relative.parts[0] != prefix
                        or member.name in names
                    ):
                        raise SystemExit("Unsafe archive member: " + member.name)
                    names.add(member.name)
                    data = archive.extractfile(member).read()
                    if secret and secret in data:
                        raise SystemExit(
                            "Private access key found in delivery artifact"
                        )
                    if path.name.endswith("-source.tar.gz"):
                        if (
                            any(
                                part
                                in (
                                    "node_modules",
                                    "bin",
                                    "data",
                                    "dist",
                                    "test-results",
                                    "playwright-report",
                                    "__pycache__",
                                )
                                for part in relative.parts[1:]
                            )
                            or relative.name == "webcp.json"
                            or relative.suffix in (".db", ".sqlite", ".log", ".pyc")
                        ):
                            raise SystemExit(
                                "Private or generated data in source archive: "
                                + member.name
                            )
                    elif relative.name == "webcp":
                        arch = "amd64" if "-amd64" in path.name else "arm64"
                        if (
                            data[:6] != b"\x7fELF\x02\x01"
                            or int.from_bytes(data[18:20], "little")
                            != {"amd64": 62, "arm64": 183}[arch]
                            or member.mode & 0o111 == 0
                        ):
                            raise SystemExit("Invalid executable architecture or mode")
                        offset, size, count = (
                            int.from_bytes(data[32:40], "little"),
                            int.from_bytes(data[54:56], "little"),
                            int.from_bytes(data[56:58], "little"),
                        )
                        if any(
                            int.from_bytes(
                                data[offset + index * size : offset + index * size + 4],
                                "little",
                            )
                            == 3
                            for index in range(count)
                        ):
                            raise SystemExit(
                                "Release executable requires an external dynamic loader"
                            )
                        if arch == host_arch:
                            native = temporary / "webcp"
                            native.write_bytes(data)
                            native.chmod(0o755)
                    elif (
                        relative.name == "webcp.example.json"
                        and json.loads(data)["accessKey"] != ""
                    ):
                        raise SystemExit(
                            "Release example includes a nonempty access key"
                        )
                if path.name.endswith("-source.tar.gz"):
                    source_path = path
                    required = (
                        "VERSION",
                        "Makefile",
                        "go.mod",
                        "go.sum",
                        "web/package-lock.json",
                        "cmd/web-panel/main.go",
                        "web/panel/manifest.json",
                    )
                else:
                    required = (
                        "webcp",
                        "README.md",
                        "LICENSE",
                        "NOTICE",
                        "webcp.example.json",
                        "DEPENDENCIES.json",
                        "THIRD_PARTY_NOTICES.generated.txt",
                    )
                if any(prefix + "/" + name not in names for name in required):
                    raise SystemExit("Release archive is missing required files")
        if native:
            info = json.loads(
                subprocess.check_output(
                    [str(native), "-version"], text=True, timeout=10
                )
            )
            if any(
                info[field] != metadata[field]
                for field in ("version", "commit", "buildTime", "goVersion")
            ):
                raise SystemExit(
                    "Executable build identity differs from release metadata"
                )
        if rebuild:
            if not native or not source_path:
                raise SystemExit(
                    "Source rebuild requires source and an executable for this Linux host"
                )
            with tarfile.open(source_path, "r:gz") as archive:
                archive.extractall(temporary / "source", filter="data")
            root = temporary / "source" / source_path.name.removesuffix(".tar.gz")
            environment = dict(os.environ, WEBCP_BUILD_COMMIT=metadata["commit"])
            if metadata["buildTime"] != "unknown":
                environment["SOURCE_DATE_EPOCH"] = str(
                    int(
                        datetime.datetime.fromisoformat(
                            metadata["buildTime"].replace("Z", "+00:00")
                        ).timestamp()
                    )
                )
            else:
                environment.pop("SOURCE_DATE_EPOCH", None)
            subprocess.run(
                ["make", "build", "ARCH=" + host_arch],
                cwd=root,
                env=environment,
                check=True,
            )
            if digest(root / "bin/webcp") != digest(native):
                raise SystemExit(
                    "Corresponding-source rebuild produced different executable bytes"
                )
    print(
        f"Verified WebCP {version}: archives, checksums, static executables, identity and private-data exclusion"
        + ("; source rebuild is byte-identical" if rebuild else "")
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--source-rebuild", action="store_true")
    parser.add_argument("--private-config", type=Path)
    args = parser.parse_args()
    verify(args.directory.resolve(), args.source_rebuild, args.private_config)


if __name__ == "__main__":
    main()
