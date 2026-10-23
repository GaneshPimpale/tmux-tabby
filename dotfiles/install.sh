#!/usr/bin/env bash
# Put this fork's personal tabby + tmux configuration in place on a new machine.
#
#   git clone https://github.com/GaneshPimpale/tmux-tabby.git ~/.tmux/plugins/tabby
#   ~/.tmux/plugins/tabby/dotfiles/install.sh
#
# What it does:
#   - clones tmux-resurrect and tmux-continuum next to tabby (if missing)
#   - installs dotfiles/config.yaml  -> ~/.config/tabby/config.yaml
#   - installs dotfiles/tmux.conf    -> ~/.tmux.conf
#     (existing files are backed up to <file>.bak.<timestamp>)
#   - builds tabby from source so the fork's patches are kept
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(dirname "$here")"
plugins="$(dirname "$repo")"
stamp="$(date +%Y%m%d%H%M%S)"

install_file() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "$dst")"
  if [ -e "$dst" ] && ! cmp -s "$src" "$dst"; then
    cp -p "$dst" "$dst.bak.$stamp"
    echo "backed up $dst -> $dst.bak.$stamp"
  fi
  cp "$src" "$dst"
  echo "installed $dst"
}

for p in tmux-resurrect tmux-continuum; do
  if [ ! -d "$plugins/$p" ]; then
    git clone --depth 1 "https://github.com/tmux-plugins/$p.git" "$plugins/$p"
  fi
done

install_file "$here/config.yaml" "${TABBY_CONFIG_DIR:-$HOME/.config/tabby}/config.yaml"
install_file "$here/tmux.conf" "$HOME/.tmux.conf"

if ! command -v go >/dev/null 2>&1; then
  echo "go is required to build tabby from source (brew install go)" >&2
  exit 1
fi
TABBY_INSTALL_FROM_SOURCE=1 "$repo/scripts/install.sh"

echo
echo "Done. Start a fresh tmux server (tmux kill-server; tmux) to load everything."
