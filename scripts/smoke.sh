#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
make check integration integration-acme build
python3 scripts/test-web.py
