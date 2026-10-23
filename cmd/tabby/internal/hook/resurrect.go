package hook

import (
	"strconv"
	"regexp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/brendandebeasi/tabby/pkg/daemon"
	"github.com/brendandebeasi/tabby/pkg/tmux"
)

// doResurrectSave replaces resurrect_save_hook.sh:
// strips Tabby utility pane lines from the resurrect save file so that
// sidebar-renderer, pane-header, and tabby-daemon panes are not restored
// as zombie shells on next resurrect-restore.
// Usage: tabby hook resurrect-save <save-file>
func doResurrectSave(args []string) {
	if len(args) < 1 {
		return
	}
	saveFile := args[0]
	if saveFile == "" || !fileExists(saveFile) {
		return
	}

	data, err := os.ReadFile(saveFile)
	if err != nil {
		return
	}

	// Ask tmux which panes are tabby chrome right now. The save file only
	// carries pane_current_command, which is "tabby" for chrome and for a
	// shell that happens to be running the CLI alike, and the start command
	// (which names the renderer) is not saved at all. The live start command
	// is unambiguous, and the pane ids let the window layouts be rewritten.
	chrome, chromeIDs := liveChromePanes()

	// Legacy process names, kept for save files written by older binaries.
	tabbyProcs := []string{
		"sidebar-renderer", "sidebar-rendere", "sidebar",
		"tabby-daemon", "pane-header",
	}

	var filtered []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 10 && fields[0] == "pane" {
			if chrome[fields[1]+"\t"+fields[2]+"\t"+fields[5]] {
				continue
			}
			cmd := fields[9]
			drop := false
			for _, proc := range tabbyProcs {
				if cmd == proc || strings.HasPrefix(cmd, proc) {
					drop = true
					break
				}
			}
			if drop {
				continue
			}
		}
		if len(fields) >= 7 && fields[0] == "window" {
			ids := chromeIDs[fields[1]+"\t"+fields[2]]
			if len(ids) > 0 {
				for i, f := range fields {
					if layoutRe.MatchString(f) {
						if rewritten, ok := stripLayoutPanes(f, ids); ok {
							fields[i] = rewritten
							line = strings.Join(fields, "\t")
						}
						break
					}
				}
			}
		}
		filtered = append(filtered, line)
	}

	os.WriteFile(saveFile, []byte(strings.Join(filtered, "\n")), 0644)
}

var layoutRe = regexp.MustCompile(`^[0-9a-f]{4},\d+x\d+,\d+,\d+`)

// liveChromePanes returns tabby's chrome panes as resurrect would key them
// (session name, window index, pane index) and their numeric pane ids per
// window (session name, window index), for layout surgery.
func liveChromePanes() (map[string]bool, map[string][]int) {
	byKey := map[string]bool{}
	byWindow := map[string][]int{}
	out, err := tmux.Cmd("list-panes", "-a", "-F",
		"#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_id}\t#{pane_start_command}").Output()
	if err != nil {
		return byKey, byWindow
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		start := f[4]
		isChrome := strings.Contains(start, "render sidebar") || strings.Contains(start, "sidebar-renderer") ||
			strings.Contains(start, "window-header") || strings.Contains(start, "pane-header")
		if !isChrome {
			continue
		}
		byKey[f[0]+"\t"+f[1]+"\t"+f[2]] = true
		if id, err := strconv.Atoi(strings.TrimPrefix(f[3], "%")); err == nil {
			wk := f[0] + "\t" + f[1]
			byWindow[wk] = append(byWindow[wk], id)
		}
	}
	return byKey, byWindow
}

// --- tmux layout strings -------------------------------------------------
//
// "csum,WxH,X,Y{...}" where a cell is a leaf "WxH,X,Y,ID", a horizontal split
// "WxH,X,Y{child,child,...}" or a vertical split "WxH,X,Y[child,...]". After
// the chrome leaves are removed, siblings are re-flowed to fill the parent
// (1-cell separators between them) and a split left with one child collapses
// into it. The checksum is tmux's layout_checksum.

type layoutCell struct {
	w, h, x, y int
	id         int // leaf pane id, -1 for a split
	kind       byte
	children   []*layoutCell
}

// stripLayoutPanes removes the given pane ids from a layout string. ok is
// false when the string could not be parsed or nothing would remain.
func stripLayoutPanes(layout string, ids []int) (string, bool) {
	comma := strings.IndexByte(layout, ',')
	if comma != 4 {
		return layout, false
	}
	pos := comma + 1
	root, err := parseLayoutCell(layout, &pos)
	if err != nil || pos != len(layout) {
		return layout, false
	}
	drop := map[int]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	root = removeLayoutCells(root, drop)
	if root == nil {
		return layout, false
	}
	body := root.String()
	return fmt.Sprintf("%04x,%s", layoutChecksum(body), body), true
}

func parseLayoutCell(s string, pos *int) (*layoutCell, error) {
	c := &layoutCell{id: -1}
	var err error
	if c.w, err = parseLayoutInt(s, pos, 'x'); err != nil {
		return nil, err
	}
	if c.h, err = parseLayoutInt(s, pos, ','); err != nil {
		return nil, err
	}
	if c.x, err = parseLayoutInt(s, pos, ','); err != nil {
		return nil, err
	}
	if c.y, err = parseLayoutInt(s, pos, 0); err != nil {
		return nil, err
	}
	if *pos >= len(s) {
		return nil, fmt.Errorf("truncated")
	}
	switch s[*pos] {
	case ',':
		*pos++
		if c.id, err = parseLayoutInt(s, pos, 0); err != nil {
			return nil, err
		}
	case '{', '[':
		c.kind = s[*pos]
		closer := byte('}')
		if c.kind == '[' {
			closer = ']'
		}
		*pos++
		for {
			child, err := parseLayoutCell(s, pos)
			if err != nil {
				return nil, err
			}
			c.children = append(c.children, child)
			if *pos >= len(s) {
				return nil, fmt.Errorf("unterminated split")
			}
			if s[*pos] == ',' {
				*pos++
				continue
			}
			if s[*pos] == closer {
				*pos++
				break
			}
			return nil, fmt.Errorf("bad split at %d", *pos)
		}
	default:
		return nil, fmt.Errorf("bad cell at %d", *pos)
	}
	return c, nil
}

// parseLayoutInt reads digits and then the expected terminator (consumed), or
// stops before any non-digit when term is 0.
func parseLayoutInt(s string, pos *int, term byte) (int, error) {
	start := *pos
	for *pos < len(s) && s[*pos] >= '0' && s[*pos] <= '9' {
		*pos++
	}
	if start == *pos {
		return 0, fmt.Errorf("expected number at %d", start)
	}
	n, _ := strconv.Atoi(s[start:*pos])
	if term != 0 {
		if *pos >= len(s) || s[*pos] != term {
			return 0, fmt.Errorf("expected %q at %d", term, *pos)
		}
		*pos++
	}
	return n, nil
}

func (c *layoutCell) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d,%d,%d", c.w, c.h, c.x, c.y)
	if c.kind == 0 {
		fmt.Fprintf(&b, ",%d", c.id)
		return b.String()
	}
	b.WriteByte(c.kind)
	for i, ch := range c.children {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(ch.String())
	}
	if c.kind == '{' {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
	return b.String()
}

// removeLayoutCells drops the leaves in drop from the tree under c and
// re-flows what is left. Returns nil when nothing remains.
func removeLayoutCells(c *layoutCell, drop map[int]bool) *layoutCell {
	if c.kind == 0 {
		if drop[c.id] {
			return nil
		}
		return c
	}
	kept := c.children[:0:0]
	for _, ch := range c.children {
		if r := removeLayoutCells(ch, drop); r != nil {
			kept = append(kept, r)
		}
	}
	switch len(kept) {
	case 0:
		return nil
	case 1:
		// The split collapses into its only child, which takes its geometry.
		fitLayoutCell(kept[0], c.w, c.h, c.x, c.y)
		return kept[0]
	}
	c.children = kept
	fitLayoutCell(c, c.w, c.h, c.x, c.y)
	return c
}

// fitLayoutCell resizes c to the given geometry. Children keep their size
// along the split axis except the last, which absorbs the difference, and
// all take the new size across it; separators are one cell.
func fitLayoutCell(c *layoutCell, w, h, x, y int) {
	c.w, c.h, c.x, c.y = w, h, x, y
	if c.kind == 0 {
		return
	}
	n := len(c.children)
	if c.kind == '{' {
		cursor := x
		for i, ch := range c.children {
			cw := ch.w
			if i == n-1 {
				cw = x + w - cursor
			}
			if cw < 1 {
				cw = 1
			}
			fitLayoutCell(ch, cw, h, cursor, y)
			cursor += cw + 1
		}
		return
	}
	cursor := y
	for i, ch := range c.children {
		chh := ch.h
		if i == n-1 {
			chh = y + h - cursor
		}
		if chh < 1 {
			chh = 1
		}
		fitLayoutCell(ch, w, chh, x, cursor)
		cursor += chh + 1
	}
}

// layoutChecksum is tmux's layout_checksum (layout-custom.c).
func layoutChecksum(s string) uint16 {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) + ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return csum
}

// doResurrectRestore replaces resurrect_restore_hook.sh:
// cleans stale Tabby state and re-initializes the sidebar after
// a tmux-resurrect restore.
// Usage: tabby hook resurrect-restore
func doResurrectRestore(args []string) {
	// 1. Kill stale Tabby processes
	for _, proc := range []string{"tabby-daemon", "sidebar-renderer", "pane-header"} {
		exec.Command("pkill", "-f", proc).Run()
	}

	// 2. Reset mouse escape sequences on all client TTYs
	resetSeq := "\033[?1000l\033[?1002l\033[?1003l\033[?1004l\033[?1006l\033[?1015l"
	out, _ := tmux.Cmd("list-clients", "-F", "#{client_tty}").Output()
	for _, tty := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if tty == "" {
			continue
		}
		f, err := os.OpenFile(tty, os.O_WRONLY, 0)
		if err != nil {
			continue
		}
		f.WriteString(resetSeq)
		f.Close()
	}

	// 3. Clean runtime files
	patterns := []string{
		daemon.RuntimeGlob(".pid"),
		daemon.RuntimeGlob(".sock"),
		"/tmp/tabby-sidebar-*.state",
		daemon.RuntimeGlob("-events.log"),
		daemon.RuntimeGlob("-input.log"),
		"/tmp/tabby-ensure-debounce-*",
	}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(pat)
		for _, m := range matches {
			os.Remove(m)
		}
	}

	// 4. Kill zombie Tabby panes
	paneOut, _ := tmux.Cmd("list-panes", "-a", "-F", "#{pane_current_command}|#{pane_id}").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(paneOut)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		cmd := parts[0]
		if cmd == "sidebar-renderer" || cmd == "sidebar" || cmd == "tabby-daemon" || cmd == "pane-header" {
			tmux.Cmd("kill-pane", "-t", parts[1]).Run()
		}
	}

	// 5. Brief pause for tmux to settle
	time.Sleep(500 * time.Millisecond)

	// 6. Re-initialize based on saved mode
	mode, _ := tmux.Cmd("show-option", "-gqv", "@tabby_sidebar").Output()
	if strings.TrimSpace(string(mode)) == "enabled" {
		sessionID, _ := tmux.Cmd("display-message", "-p", "#{session_id}").Output()
		windowID, _ := tmux.Cmd("display-message", "-p", "#{window_id}").Output()
		sid := strings.TrimSpace(string(sessionID))
		wid := strings.TrimSpace(string(windowID))
		if sid != "" {
			go ensureSidebar(sid, wid)
		}
	}
}
