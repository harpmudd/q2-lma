#!/bin/sh
# q2-lma for the Shanling Q2: static Go for 32-bit little-endian MIPS with soft float, staged with
# its card files in OUT/.lma and zipped as OUT/q2-lma-VERSION-SHA.zip, to unzip at the card's root.
#   contrib/shanlingq2/build.sh [OUT]
# Needs Go 1.27 and Python 3. Runs under Git Bash on Windows.
set -eu
APP=lma
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
OUT=${1:-$ROOT/target/shanlingq2}
GO=${GO:-$(command -v go || echo "/c/Program Files/Go/bin/go.exe")}
if [ -z "${PY:-}" ]; then # the first that runs: Windows' python3 may be a Store stub
    for PY in python3 python ''; do [ -n "$PY" ] && "$PY" -c '' 2>/dev/null && break; done
fi
[ -n "$PY" ] || { echo "build.sh: no working python3 or python" >&2; exit 1; }
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
VERSION=$(sed -n 's/^\tversion *= "q2-\([a-z0-9.-]*\)"$/\1/p' "$ROOT/cmd/q2-$APP/main.go")
[ -n "$VERSION" ] || { echo "build.sh: no version constant in cmd/q2-$APP/main.go" >&2; exit 1; }
SHA=$(git -C "$ROOT" rev-parse --short=7 HEAD 2>/dev/null || echo local)

R="$OUT/.$APP"
rm -rf "$R"
mkdir -p "$R"
cd "$ROOT"
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
    "$GO" build -trimpath -ldflags='-s -w -buildid=' -o "$R/q2-$APP" "./cmd/q2-$APP"
cp "$HERE/launcher.sh" "$R/run"
cp "$HERE/q2app" "$R/"
for i in icon.png icon-40.png; do [ ! -f "$HERE/$i" ] || cp "$HERE/$i" "$R/"; done
rm -f "$OUT"/q2-$APP-*.zip
"$PY" -I "$HERE/package.py" "$R" "$OUT/q2-$APP-$VERSION-$SHA.zip"
