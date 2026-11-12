#!/usr/bin/env python3
"""Collect installed dependencies and their original notices without local paths."""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parent.parent
PATTERNS = (
    "LICENSE*",
    "LICENCE*",
    "COPYING*",
    "NOTICE*",
    "COPYRIGHT*",
    "PATENTS*",
    "license*",
    "licence*",
)


def objects(source):
    decoder = json.JSONDecoder()
    offset = 0
    while offset < len(source):
        if source[offset].isspace():
            offset += 1
            continue
        value, length = decoder.raw_decode(source[offset:])
        offset += length
        yield value


def notice_files(directory):
    return sorted(
        path
        for path in directory.iterdir()
        if path.is_file()
        and path.name.lower().startswith(
            (
                "license",
                "licence",
                "copying",
                "notice",
                "copyright",
                "patents",
                "thirdpartynotices",
            )
        )
    )


def generate():
    if not (ROOT / "web/dist/index.html").is_file():
        raise SystemExit(
            "Run make web-build before collecting backend dependency notices"
        )
    inventory, notices, missing = [], [], []
    source = subprocess.check_output(
        ["go", "list", "-deps", "-json", "./cmd/web-panel"], cwd=ROOT, text=True
    )
    modules = {}
    for package in objects(source):
        module = package.get("Module", {})
        if not module or module.get("Main"):
            continue
        effective = module.get("Replace", module)
        modules[module["Path"]] = (
            effective["Version"],
            Path(effective["Dir"]),
            effective["Path"],
        )
    goroot = Path(
        subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, text=True).strip()
    )
    modules["Go standard library"] = (
        subprocess.check_output(
            ["go", "env", "GOVERSION"], cwd=ROOT, text=True
        ).strip(),
        goroot,
        "https://go.dev",
    )
    components = [
        ("go", name, release, directory, {"source": effective})
        for name, (release, directory, effective) in sorted(modules.items())
    ]
    lock = json.loads((ROOT / "web/package-lock.json").read_text())
    for package_path, record in sorted(lock["packages"].items()):
        if not package_path:
            continue
        directory = ROOT / "web" / package_path
        if not directory.is_dir():
            if record.get("optional"):
                continue
            raise SystemExit(f"Run npm ci before collecting notices: {package_path}")
        package = json.loads((directory / "package.json").read_text())
        components.append(
            (
                "npm",
                package.get("name", package_path),
                record["version"],
                directory,
                {
                    "license": package.get("license", record.get("license", "unknown")),
                    "author": package.get("author"),
                    "development": bool(record.get("dev")),
                    "resolved": record.get("resolved"),
                    "integrity": record.get("integrity"),
                },
            )
        )
    for ecosystem, name, release, directory, metadata in components:
        files = notice_files(directory)
        if not files:
            identifier = metadata.get("license")
            canonical = (
                ROOT / "docs/licenses" / f"{identifier}.txt"
                if isinstance(identifier, str)
                else None
            )
            if ecosystem != "npm" or canonical is None or not canonical.is_file():
                missing.append(f"{ecosystem}: {name}@{release}")
            else:
                metadata["noticeSource"] = (
                    "package.json declares license; package omits separate license text; canonical terms bundled"
                )
                attribution = json.dumps(metadata.get("author"), ensure_ascii=False)
                notices.append(
                    f"===== npm: {name}@{release} / declared metadata =====\nAuthor metadata: {attribution}\nLicense: {identifier}\nSource archive: {metadata.get('resolved')}\n\n"
                    + canonical.read_text()
                    + "\n"
                )
        record = {
            "ecosystem": ecosystem,
            "name": name,
            "version": release,
            **metadata,
            "notices": [],
        }
        for path in files:
            data = path.read_bytes()
            record["notices"].append(
                {"file": path.name, "sha256": hashlib.sha256(data).hexdigest()}
            )
            notices.append(
                f"===== {ecosystem}: {name}@{release} / {path.name} =====\n\n"
                + data.decode("utf-8", errors="replace")
                + "\n"
            )
        inventory.append(record)
    if missing:
        raise SystemExit("Missing original license text:\n" + "\n".join(missing))
    return (
        json.dumps(
            {
                "format": 1,
                "scope": "linked Go modules and installed frontend build dependency graph; not all listed npm packages are embedded",
                "components": inventory,
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        "\n".join(notices),
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--sync-dist", action="store_true")
    args = parser.parse_args()
    inventory, notices = generate()
    output = ROOT / "web/public/licenses"
    for name, data in [
        ("DEPENDENCIES.json", inventory),
        ("THIRD_PARTY_NOTICES.generated.txt", notices),
    ]:
        target = output / name
        if args.check:
            if not target.exists() or target.read_bytes().decode("utf-8") != data:
                raise SystemExit(
                    f"Dependency notices are stale: {target.relative_to(ROOT)}"
                )
            embedded = ROOT / "web/dist/licenses" / name
            if not embedded.is_file() or embedded.read_bytes().decode("utf-8") != data:
                raise SystemExit(
                    "Embedded dependency notices are stale: run make web-build"
                )
        else:
            target.write_text(data)
    if args.sync_dist:
        destination = ROOT / "web/dist/licenses"
        if not (ROOT / "web/dist/index.html").is_file():
            raise SystemExit("Build the frontend before syncing embedded notices")
        destination.mkdir(parents=True, exist_ok=True)
        for path in output.iterdir():
            if path.is_file():
                shutil.copyfile(path, destination / path.name)
    print(
        "Verified dependency notices" if args.check else "Generated dependency notices"
    )


if __name__ == "__main__":
    main()
