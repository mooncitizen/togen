#!/bin/sh
# Installs the togen binary from the latest GitHub release.
#
#   curl -fsSL https://mooncitizen.github.io/togen/install.sh | sh
#
# TOGEN_VERSION      tag to install, default the latest release
# TOGEN_INSTALL_DIR  where the binary lands, default $HOME/.local/bin
# --no-profile       never edit a shell profile

set -eu

repo="mooncitizen/togen"
version="${TOGEN_VERSION:-}"
install_dir="${TOGEN_INSTALL_DIR:-$HOME/.local/bin}"
edit_profile=1

for arg in "$@"; do
  case "$arg" in
    --no-profile) edit_profile=0 ;;
    *) echo "unknown option: $arg" >&2; exit 1 ;;
  esac
done

die() {
  echo "error: $1" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

fetch() {
  if have curl; then
    curl -fsSL "$1"
  elif have wget; then
    wget -qO- "$1"
  else
    die "neither curl nor wget is installed"
  fi
}

fetch_to() {
  if have curl; then
    curl -fsSL -o "$2" "$1"
  else
    wget -qO "$2" "$1"
  fi
}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "togen has no build for $(uname -s); build from source instead" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "togen has no build for $(uname -m)" ;;
esac

if [ -z "$version" ]; then
  version=$(fetch "https://api.github.com/repos/$repo/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$version" ] || die "could not read the latest release tag from the GitHub API"
fi
case "$version" in
  v*) tag="$version" ;;
  *) tag="v$version" ;;
esac

archive="togen_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$tag"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "Downloading togen $tag for $os/$arch"
fetch_to "$base/$archive" "$work/$archive" || die "could not download $base/$archive"
fetch_to "$base/checksums.txt" "$work/checksums.txt" || die "could not download $base/checksums.txt"

want=$(grep " $archive\$" "$work/checksums.txt" | awk '{print $1}')
[ -n "$want" ] || die "checksums.txt has no entry for $archive"

if have sha256sum; then
  got=$(sha256sum "$work/$archive" | awk '{print $1}')
elif have shasum; then
  got=$(shasum -a 256 "$work/$archive" | awk '{print $1}')
else
  die "neither sha256sum nor shasum is installed, so the download cannot be verified"
fi
[ "$got" = "$want" ] || die "$archive does not match its checksum ($got, want $want)"

tar xzf "$work/$archive" -C "$work" togen || die "the archive has no togen entry"
mkdir -p "$install_dir"
chmod 0755 "$work/togen"
mv "$work/togen" "$install_dir/togen"

echo "Installed togen $tag to $install_dir/togen"

case ":$PATH:" in
  *":$install_dir:"*) exit 0 ;;
esac

if [ "$edit_profile" -eq 0 ]; then
  echo "$install_dir is not on your PATH. Add it, or run $install_dir/togen directly."
  exit 0
fi

case "${SHELL:-}" in
  */zsh) profile="$HOME/.zshrc" ;;
  */bash) profile="$HOME/.bashrc" ;;
  *) profile="$HOME/.profile" ;;
esac

marker="# added by togen install"
if [ -f "$profile" ] && grep -qF "$marker" "$profile"; then
  echo "$install_dir is not on your PATH, but $profile already exports it. Open a new shell."
  exit 0
fi

{
  echo ""
  echo "$marker"
  echo "export PATH=\"$install_dir:\$PATH\""
} >> "$profile"

echo "Added $install_dir to your PATH in $profile."
echo "Open a new shell, or run: . $profile"
