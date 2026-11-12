#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
make check integration build
python3 scripts/test-web.py
