#!/usr/bin/env bash
#
# install.sh — one-liner installer for gh-aw-fleet on Linux and macOS.
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/rshade/gh-aw-fleet/main/install.sh | bash
#
# Environment variables:
#   VERSION       Release tag to install (default: latest, e.g. "v0.2.0").
#   INSTALL_DIR   Directory to install the binary into
#                 (default: $HOME/.local/bin).
#
# Internal-only (used by CI for the checksum-tamper test; not part of the
# public interface):
#   RELEASE_BASE_URL  Override the release asset base URL. Defaults to
#                     https://github.com/rshade/gh-aw-fleet/releases/download
#
# This script intentionally avoids editing your shell profile. If the install
# directory is not on $PATH, the script prints a copy-pasteable export line
# on stderr and exits successfully.

set -euo pipefail
IFS=$'\n\t'

REPO="rshade/gh-aw-fleet"
PROJECT="gh-aw-fleet"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"
DEFAULT_BASE_URL="https://github.com/${REPO}/releases/download"

TMPDIR_INSTALL=""
cleanup() { [ -n "$TMPDIR_INSTALL" ] && rm -rf "$TMPDIR_INSTALL"; }
trap cleanup EXIT

log()  { printf '%s\n' "$*" >&2; }
die()  { log "error: $*"; exit 1; }

detect_platform() {
    local os arch
    os=$(uname -s)
    case "$os" in
        Linux)   os=linux ;;
        Darwin)  os=darwin ;;
        MINGW*|MSYS*|CYGWIN*)
            log "Windows-flavored shell detected ($os)."
            log "Use install.ps1 from PowerShell instead:"
            log "  iwr -UseBasicParsing https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
            exit 1
            ;;
        *) die "unsupported OS: $os" ;;
    esac

    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   arch=amd64 ;;
        aarch64|arm64)  arch=arm64 ;;
        *) die "unsupported architecture: $arch" ;;
    esac

    printf '%s_%s\n' "$os" "$arch"
}

resolve_version() {
    if [ -n "${VERSION:-}" ]; then
        printf '%s\n' "$VERSION"
        return
    fi
    local api="https://api.github.com/repos/${REPO}/releases/latest"
    local body tag token
    token="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
    if [ -n "$token" ]; then
        body=$(curl -fsSL -H "Authorization: Bearer ${token}" "$api") \
            || die "failed to fetch latest release from $api"
    else
        body=$(curl -fsSL "$api") || die "failed to fetch latest release from $api"
    fi
    tag=$(printf '%s\n' "$body" \
        | grep -E '^[[:space:]]*"tag_name":' \
        | head -n1 \
        | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
    [ -n "$tag" ] || die "could not parse tag_name from $api"
    printf '%s\n' "$tag"
}

sha256_check() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c -
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 -c -
    else
        die "neither sha256sum nor shasum is available"
    fi
}

download_and_verify() {
    local version=$1 platform=$2 tmpdir=$3
    local version_strip="${version#v}"
    local archive="${PROJECT}_${version_strip}_${platform}.tar.gz"
    local checksums="${PROJECT}_${version_strip}_checksums.txt"
    local base="${RELEASE_BASE_URL:-$DEFAULT_BASE_URL}/${version}"

    log "Downloading ${archive}"
    curl -fsSL "${base}/${archive}"   -o "${tmpdir}/${archive}" \
        || die "failed to download ${base}/${archive}"
    log "Downloading ${checksums}"
    curl -fsSL "${base}/${checksums}" -o "${tmpdir}/${checksums}" \
        || die "failed to download ${base}/${checksums}"

    log "Verifying SHA-256"
    local archive_re="${archive//./\\.}"
    (
        cd "$tmpdir"
        grep -E "[[:space:]]${archive_re}\$" "${checksums}" | sha256_check
    ) >&2 || die "checksum verification failed for ${archive}"

    printf '%s\n' "${tmpdir}/${archive}"
}

install_binary() {
    local archive=$1 install_dir=$2
    mkdir -p "$install_dir"
    tar -xzf "$archive" -C "$install_dir" "$PROJECT"
    chmod +x "${install_dir}/${PROJECT}"
    log "Installed ${install_dir}/${PROJECT}"
}

warn_if_not_in_path() {
    local install_dir=$1
    case ":$PATH:" in
        *":$install_dir:"*) return 0 ;;
    esac
    log ""
    log "Note: ${install_dir} is not on your \$PATH."
    log "To use gh-aw-fleet from any shell, add this line to your shell profile:"
    log ""
    log "  export PATH=\"${install_dir}:\$PATH\""
    log ""
}

main() {
    local platform version archive install_dir
    platform=$(detect_platform)
    version=$(resolve_version)
    install_dir="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

    log "Installing ${PROJECT} ${version} (${platform}) into ${install_dir}"

    TMPDIR_INSTALL=$(mktemp -d 2>/dev/null || mktemp -d -t gh-aw-fleet)

    archive=$(download_and_verify "$version" "$platform" "$TMPDIR_INSTALL")
    install_binary "$archive" "$install_dir"
    warn_if_not_in_path "$install_dir"

    log ""
    log "Run 'gh-aw-fleet --help' to get started."
}

main "$@"
