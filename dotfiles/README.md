# Personal dotfiles

My tabby + tmux configuration, kept here so it can be installed on any machine
alongside this fork.

| File          | Installed to                  | Notes |
|---------------|-------------------------------|-------|
| `config.yaml` | `~/.config/tabby/config.yaml` | Plain vertical tab list: single transparent `Default` group, all colours inherit the terminal palette (`default`), every button/indicator/widget off, no tabby keybindings. Only `terminal_title` is enabled, so kitty's tab title shows the running program. |
| `tmux.conf`   | `~/.tmux.conf`                | Mouse on, `@tabby_base_index 1`, tmux-resurrect + tmux-continuum (15 min autosave, auto-restore) loaded before tabby. |

## Install

```sh
git clone https://github.com/GaneshPimpale/tmux-tabby.git ~/.tmux/plugins/tabby
~/.tmux/plugins/tabby/dotfiles/install.sh
```

The script clones resurrect/continuum if missing, copies both files into place
(backing up anything it would overwrite), and builds tabby from source with
`TABBY_INSTALL_FROM_SOURCE=1` so the fork's patches survive. Go is required.

## Caveats

- tabby's `SaveConfig` rewrites `config.yaml` without comments whenever a menu
  action changes a setting. Re-copy from here if that happens.
- After editing theme keys, restart the daemon (kill the pid in
  `/tmp/tabby-daemon-<session>.pid`; the watchdog respawns it) rather than
  re-sourcing `~/.tmux.conf`.
- Keep the config in sync: `cp ~/.config/tabby/config.yaml dotfiles/` and
  `cp ~/.tmux.conf dotfiles/tmux.conf`, then commit.
