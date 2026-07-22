#!/bin/sh
# tgc installer — downloads a release binary from GitHub (no Go required).
#   curl -fsSL https://raw.githubusercontent.com/grigoreo-dev/tgc/main/install.sh | sh
# Env: TGC_VERSION=vX.Y.Z, TGC_INSTALL_DIR=/path, TGC_NO_SETUP=1, GITHUB_TOKEN / GH_TOKEN
set -eu

REPO="grigoreo-dev/tgc"
BINARY="tgc"

err() { printf 'error: %s\n' "$1" >&2; exit 1; }
info() { printf '%s\n' "$1" >&2; }

# --- dependencies ---
have() { command -v "$1" >/dev/null 2>&1; }
have tar || err "tar is required"
if have curl; then DL="curl"; elif have wget; then DL="wget"; else err "curl or wget is required"; fi
if have sha256sum; then SHA="sha256sum"; elif have shasum; then SHA="shasum -a 256"; else err "sha256sum or shasum is required"; fi

# --- detect platform ---
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) os="linux" ;;
  darwin) os="darwin" ;;
  *) err "unsupported OS: $os" ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- auth header for GitHub API ---
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
gh_get() {
  # $1 = url, $2 = output ('-' for stdout)
  if [ "$DL" = "curl" ]; then
    if [ -n "$TOKEN" ]; then
      curl -fsSL --proto '=https' --tlsv1.2 -H "Authorization: Bearer $TOKEN" "$1" ${2:+-o "$2"}
    else
      curl -fsSL --proto '=https' --tlsv1.2 "$1" ${2:+-o "$2"}
    fi
  else
    if [ -n "$TOKEN" ]; then
      wget -q --header="Authorization: Bearer $TOKEN" -O "${2:--}" "$1"
    else
      wget -q -O "${2:--}" "$1"
    fi
  fi
}

# --- resolve version ---
VERSION="${TGC_VERSION:-}"
if [ -z "$VERSION" ]; then
  info "Resolving latest release..."
  VERSION="$(gh_get "https://api.github.com/repos/${REPO}/releases/latest" - \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
  [ -n "$VERSION" ] || err "could not resolve latest version (rate limited? set GITHUB_TOKEN)"
fi
NUM="${VERSION#v}"

ASSET="${BINARY}_${NUM}_${os}_${arch}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"

# --- download + verify ---
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
info "Downloading ${ASSET} (${VERSION})..."
gh_get "${BASE}/${ASSET}" "${TMP}/${ASSET}" || err "download failed for ${ASSET}"
gh_get "${BASE}/checksums.txt" "${TMP}/checksums.txt" || err "download failed for checksums.txt"

info "Verifying checksum..."
EXPECTED="$(awk -v f="$ASSET" '$2==f {print $1}' "${TMP}/checksums.txt")"
[ -n "$EXPECTED" ] || err "no checksum entry for ${ASSET}"
ACTUAL="$(cd "$TMP" && $SHA "$ASSET" | awk '{print $1}')"
[ "$EXPECTED" = "$ACTUAL" ] || err "checksum mismatch for ${ASSET}"

tar -xzf "${TMP}/${ASSET}" -C "$TMP"
[ -f "${TMP}/${BINARY}" ] || err "binary not found in archive"
chmod +x "${TMP}/${BINARY}"

# --- choose install dir ---
DIR="${TGC_INSTALL_DIR:-$HOME/.local/bin}"
if [ "$DIR" = "/usr/local/bin" ] && [ ! -w "$DIR" ]; then
  if have sudo && [ -e /dev/tty ]; then
    info "Installing to /usr/local/bin (sudo)..."
    # SC2024: the </dev/tty redirect is intentionally applied by the current
    # user (who owns the tty), not root — it feeds sudo's password prompt when
    # this script runs via `curl | sh` and stdin is the pipe. Not a bug.
    # shellcheck disable=SC2024
    sudo mv "${TMP}/${BINARY}" "${DIR}/${BINARY}" </dev/tty || err "sudo install failed"
  else
    err "cannot write ${DIR}; re-run with TGC_INSTALL_DIR=\$HOME/.local/bin or install manually"
  fi
else
  mkdir -p "$DIR"
  mv "${TMP}/${BINARY}" "${DIR}/${BINARY}"
fi
info "Installed ${BINARY} ${VERSION} to ${DIR}/${BINARY}"

# --- PATH hint (fallback when setup is skipped or fails) ---
path_hint() {
  case ":$PATH:" in
    *":${DIR}:"*) ;;
    *)
      info ""
      info "Add ${DIR} to your PATH:"
      info "    export PATH=\"${DIR}:\$PATH\""
      ;;
  esac
}

# --- post-install: PATH + shell completion via tgc self setup ---
# Opt out with TGC_NO_SETUP=1 (binary install still succeeds).
if [ "${TGC_NO_SETUP:-}" = "1" ]; then
  info "Skipping self setup (TGC_NO_SETUP=1)."
  path_hint
else
  info "Configuring PATH and shell completion..."
  if ! "$DIR/$BINARY" self setup; then
    info "warning: tgc self setup failed; binary install succeeded."
    info "Configure manually:"
    info "    tgc self setup"
    info "    tgc completion <shell>   # generate script to stdout"
    path_hint
  fi
fi
