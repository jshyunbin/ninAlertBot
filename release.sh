#!/usr/bin/env bash
#
# release.sh — build, package, tag, and publish a ninAlertBot release.
#
# Usage:
#   ./release.sh v0.3.0            # full release: test, build, tag, push, gh release
#   ./release.sh v0.3.0 --dry-run  # build + package into dist/ only; no tag/push/publish
#
# Requires: go, zip, git, gh (authenticated). Run from a clean main checkout.

set -euo pipefail

VERSION="${1:-}"
DRY_RUN="false"
[[ "${2:-}" == "--dry-run" ]] && DRY_RUN="true"

die() { echo "error: $*" >&2; exit 1; }

# --- validate input ---------------------------------------------------------
[[ -n "$VERSION" ]] || die "usage: ./release.sh vX.Y.Z [--dry-run]"
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "version must look like v1.2.3 (got '$VERSION')"

command -v go  >/dev/null || die "go not found"
command -v zip >/dev/null || die "zip not found"
command -v git >/dev/null || die "git not found"

cd "$(dirname "$0")"

# --- preflight checks -------------------------------------------------------
if [[ "$DRY_RUN" == "false" ]]; then
  command -v gh >/dev/null || die "gh not found (needed to publish; use --dry-run to skip)"
  gh auth status >/dev/null 2>&1 || die "gh is not authenticated; run: gh auth login"

  branch="$(git rev-parse --abbrev-ref HEAD)"
  [[ "$branch" == "main" ]] || die "must be on 'main' (on '$branch')"
  [[ -z "$(git status --porcelain)" ]] || die "working tree not clean; commit or stash first"
  git fetch --quiet origin
  [[ "$(git rev-parse @)" == "$(git rev-parse '@{u}')" ]] || die "local main differs from origin/main; push/pull first"
  git rev-parse "$VERSION" >/dev/null 2>&1 && die "tag $VERSION already exists"
fi

# --- test -------------------------------------------------------------------
echo ">> running tests"
go vet ./...
go test ./...

# --- build + package --------------------------------------------------------
echo ">> building $VERSION"
rm -rf dist && mkdir -p dist

# platform list: "GOOS GOARCH binary-extension"
PLATFORMS=(
  "windows amd64 .exe"
  "windows arm64 .exe"
  "linux   amd64 ''"
  "darwin  arm64 ''"
  "darwin  amd64 ''"
)

for entry in "${PLATFORMS[@]}"; do
  read -r goos goarch ext <<<"$entry"
  ext="${ext//\'/}"  # strip the placeholder quotes for empty ext
  name="ninalertbot-${VERSION}-${goos}-${goarch}"
  d="dist/${name}"
  mkdir -p "$d"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${d}/ninalertbot${ext}" ./cmd/ninalertbot
  cp README.md README.ko.md config.example.yaml "$d/"
  ( cd dist && zip -q -r "${name}.zip" "${name}" && rm -rf "${name}" )
  echo "   packaged ${name}.zip"
done

( cd dist && shasum -a 256 ./*.zip > SHA256SUMS.txt )
echo ">> artifacts in dist/:"
ls -1 dist/

if [[ "$DRY_RUN" == "true" ]]; then
  echo ">> dry run complete — nothing tagged or published"
  exit 0
fi

# --- tag + publish ----------------------------------------------------------
echo ">> tagging and pushing $VERSION"
git tag -a "$VERSION" -m "ninAlertBot $VERSION"
git push origin "$VERSION"

echo ">> creating GitHub release"
gh release create "$VERSION" \
  --title "ninAlertBot $VERSION" \
  --generate-notes \
  dist/*.zip dist/SHA256SUMS.txt

echo ">> done: $(gh release view "$VERSION" --json url --jq .url)"
