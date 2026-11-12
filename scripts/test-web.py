#!/usr/bin/env python3
"""Run browser regression with temporary data, credentials, ports and owned containers."""

import argparse
import contextlib
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent


def free_ports(count):
    with contextlib.ExitStack() as stack:
        sockets = [stack.enter_context(socket.socket()) for _ in range(count)]
        for connection in sockets:
            connection.bind(("127.0.0.1", 0))
        return [connection.getsockname()[1] for connection in sockets]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dev", action="store_true")
    parser.add_argument("--binary", type=Path, default=ROOT / "bin/webcp")
    args, filters = parser.parse_known_args()
    if not args.binary.is_file():
        raise SystemExit("Build WebCP first: make build")
    with tempfile.TemporaryDirectory(prefix="webcp-browser-") as directory:
        data = Path(directory).resolve()
        management, http, https = free_ports(3)
        key = secrets.token_urlsafe(32)
        environment = dict(os.environ)
        environment.pop("WEBCP_ACCESS_KEY_FILE", None)
        environment.update(WEBCP_ACCESS_KEY=key, WEBCP_TEST_ACCESS_KEY=key)
        base = f"http://127.0.0.1:{management}"
        environment.update(
            RESTY_DEV_API=base,
            RESTY_TEST_URL="http://127.0.0.1:5173" if args.dev else base,
            RESTY_TEST_WEBSITE_URL=f"http://127.0.0.1:{http}",
            RESTY_TEST_MODE="dev" if args.dev else "production",
        )
        container = "webcp-browser-" + secrets.token_hex(6)
        command = [
            str(args.binary.resolve()),
            "-listen",
            f"127.0.0.1:{management}",
            "-data",
            str(data),
            "-container",
            container,
            "-http-bind",
            f"127.0.0.1:{http}",
            "-https-bind",
            f"127.0.0.1:{https}",
        ]
        with (data / "manager.log").open("wb") as log:
            process = subprocess.Popen(
                command, cwd=ROOT, env=environment, stdout=log, stderr=subprocess.STDOUT
            )
            try:
                deadline = time.monotonic() + 30
                while True:
                    try:
                        with urllib.request.urlopen(
                            base + "/api/auth/status", timeout=1
                        ) as response:
                            json.load(response)
                        break
                    except (OSError, ValueError):
                        if process.poll() is not None or time.monotonic() > deadline:
                            raise RuntimeError(
                                "Isolated backend failed: "
                                + (data / "manager.log").read_text()
                            )
                        time.sleep(0.1)
                subprocess.run(
                    ["npm", "run", "test:e2e", "--", *filters],
                    cwd=ROOT / "web",
                    env=environment,
                    check=True,
                )
            finally:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                # Select by the private data-root label, never by a user's container name.
                found = subprocess.run(
                    [
                        "docker",
                        "ps",
                        "-aq",
                        "--filter",
                        "label=resty.manager.root=" + str(data),
                    ],
                    capture_output=True,
                    text=True,
                    check=True,
                )
                identifiers = found.stdout.split()
                if identifiers:
                    subprocess.run(
                        ["docker", "rm", "-f", *identifiers],
                        check=True,
                        stdout=subprocess.DEVNULL,
                    )


if __name__ == "__main__":
    main()
