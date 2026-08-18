#!/usr/bin/env bash
# Update the Go toolchain used by this repo to the latest stable patch release.
#
# Touches two places that must stay in lockstep:
#   - go.mod "go" directive  — also selects the toolchain CI installs, because the
#     workflows use actions/setup-go with go-version-file: go.mod
#   - Dockerfile builder FROM — the toolchain the released image is actually built with
#
# Why this script exists rather than leaving it to Renovate: the Dockerfile used to pin
# golang:1.26.4-alpine3.22, and upstream never published 1.26.5+ for the alpine3.22
# variant line. A pinned-variant bump therefore had no candidate, Renovate offered
# nothing, and the builder silently sat on a Go with known stdlib CVEs. So this script
# re-discovers the newest available alpine variant instead of assuming the current one
# still receives builds.
#
# Usage:
#   hack/update-go-toolchain.sh            # apply changes
#   hack/update-go-toolchain.sh --check    # report only, exit 1 if an update is available
#
# Outputs (when run under GitHub Actions) to $GITHUB_OUTPUT:
#   current_go, latest_go, updated (true|false), image
set -euo pipefail

cd "$(dirname "$0")/.."

CHECK_ONLY=false
[[ "${1:-}" == "--check" ]] && CHECK_ONLY=true

emit() { [[ -n "${GITHUB_OUTPUT:-}" ]] && echo "$1=$2" >>"$GITHUB_OUTPUT" || true; }
log()  { echo "==> $*" >&2; }

current_go="$(sed -nE 's/^go ([0-9]+\.[0-9]+(\.[0-9]+)?)$/\1/p' go.mod | head -1)"
[[ -n "$current_go" ]] || { echo "could not parse 'go' directive from go.mod" >&2; exit 1; }

# Latest stable Go. go.dev/dl marks prereleases stable:false, so this never returns an rc.
latest_go="$(
  curl -fsSL --retry 3 'https://go.dev/dl/?mode=json' |
    python3 -c '
import json,sys
rs=[r["version"][2:] for r in json.load(sys.stdin) if r.get("stable")]
def key(v): return [int(x) for x in (v.split(".")+["0","0"])[:3]]
print(sorted(rs,key=key)[-1] if rs else "")'
)"
[[ -n "$latest_go" ]] || { echo "could not determine latest stable Go release" >&2; exit 1; }

log "go.mod toolchain: $current_go"
log "latest stable Go: $latest_go"
emit current_go "$current_go"
emit latest_go  "$latest_go"

# Newest alpine variant published for $latest_go. Pin to a concrete alpineX.Y rather
# than the floating "-alpine" tag so the base OS moves as a reviewable, deliberate change.
variant="$(
  python3 - "$latest_go" <<'PY'
import json,sys,urllib.request
go=sys.argv[1]; out=[]
url=f"https://hub.docker.com/v2/repositories/library/golang/tags/?page_size=100&name={go}-alpine"
while url:
    with urllib.request.urlopen(url, timeout=30) as r:
        d=json.load(r)
    out+=[t["name"] for t in d.get("results",[])]
    url=d.get("next")
import re
vs=[]
for n in out:
    m=re.fullmatch(rf"{re.escape(go)}-alpine(\d+)\.(\d+)", n)
    if m: vs.append(((int(m.group(1)),int(m.group(2))), n))
print(sorted(vs)[-1][1] if vs else "")
PY
)"
if [[ -z "$variant" ]]; then
  # No pinned alpineX.Y build for this patch yet (they can lag a few hours).
  log "no alpineX.Y variant published for $latest_go yet; leaving Dockerfile untouched"
  emit updated false
  exit 0
fi

digest="$(
  python3 - "$variant" <<'PY'
import json,sys,urllib.request
tag=sys.argv[1]
tok=json.load(urllib.request.urlopen(
    "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/golang:pull",
    timeout=30))["token"]
req=urllib.request.Request(
    f"https://registry-1.docker.io/v2/library/golang/manifests/{tag}",
    headers={"Authorization":f"Bearer {tok}",
             "Accept":"application/vnd.oci.image.index.v1+json,"
                      "application/vnd.docker.distribution.manifest.list.v2+json"})
with urllib.request.urlopen(req, timeout=30) as r:
    print(r.headers.get("docker-content-digest",""))
PY
)"
[[ "$digest" == sha256:* ]] || { echo "could not resolve digest for golang:$variant" >&2; exit 1; }

image="golang:${variant}@${digest}"
log "target builder image: $image"
emit image "$image"

current_from="$(sed -nE 's/^FROM .*(golang:[^ ]+) AS builder$/\1/p' Dockerfile | head -1)"

if [[ "$current_go" == "$latest_go" && "$current_from" == "golang:${variant}@${digest}" ]]; then
  log "already up to date"
  emit updated false
  exit 0
fi

if [[ "$CHECK_ONLY" == true ]]; then
  log "update available: Go $current_go -> $latest_go; builder -> $image"
  emit updated true
  exit 1
fi

# go.mod: bump the directive, then let the toolchain normalise the file.
python3 - "$latest_go" <<'PY'
import pathlib,re,sys
p=pathlib.Path("go.mod")
p.write_text(re.sub(r"^go [0-9]+\.[0-9]+(\.[0-9]+)?$", f"go {sys.argv[1]}",
                    p.read_text(), count=1, flags=re.M))
PY

# Dockerfile: replace the builder base, preserving the rest of the line.
python3 - "$image" <<'PY'
import pathlib,re,sys
p=pathlib.Path("Dockerfile")
p.write_text(re.sub(r"(^FROM .*)golang:[^ ]+( AS builder$)",
                    lambda m: m.group(1)+sys.argv[1]+m.group(2),
                    p.read_text(), count=1, flags=re.M))
PY

command -v go >/dev/null && go mod tidy

log "updated Go $current_go -> $latest_go and builder base -> $image"
emit updated true
