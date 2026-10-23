package daemon

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/brendandebeasi/tabby/pkg/tmux"
)

// tmuxCmd builds a tmux *exec.Cmd bounded by tmuxCmdTimeout.
//
// Every tmux invocation in the daemon must be bounded: an unbounded
// fork/exec on the reconcile path blocks the event loop indefinitely when
// the tmux server stalls (macOS sleep/wake), which starves the heartbeat
// and trips the deadlock watchdog.
//
// The context's cancel func is released by Cmd.Cancel, which os/exec runs
// once the process exits, so a completed command drops its timer
// immediately rather than holding it for the full timeout. WaitDelay
// bounds the tail case where the process dies but an inherited pipe is
// still held open by a child.
func tmuxCmd(args ...string) *exec.Cmd {
	// Close the hook gate before a mutating command can fire tabby's hooks
	// back at this daemon. Done at build time rather than at Run time: callers
	// build then immediately run, and muting early is the safe direction.
	noteTmuxMutation(args)
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCmdTimeout)
	cmd := tmux.CmdContext(ctx, args...)
	cmd.Cancel = func() error {
		defer cancel()
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = time.Second
	// Keep cancel reachable for vet: Cmd.Cancel above owns the call, and
	// the context is bounded by its own timeout regardless.
	_ = cancel
	return cmd
}

// @tabby_spawning is a cross-process bracket (newwindow and the layout pass
// raise it while panes are being created) that half a dozen housekeeping
// passes consult before touching panes. Each consult was its own tmux
// round-trip (~10ms on macOS) on every tick. Cache the answer briefly: one
// read serves a whole housekeeping pass, and the daemon's own writes update
// the cache directly.
var (
	spawningMu   sync.Mutex
	spawningVal  bool
	spawningAt   time.Time
	spawningTTL  = 150 * time.Millisecond
)

// spawningActive reports whether the @tabby_spawning bracket is raised.
func spawningActive() bool {
	spawningMu.Lock()
	defer spawningMu.Unlock()
	if time.Since(spawningAt) < spawningTTL {
		return spawningVal
	}
	out, err := tmuxCmd("show-option", "-gqv", "@tabby_spawning").Output()
	spawningVal = err == nil && strings.TrimSpace(string(out)) == "1"
	spawningAt = time.Now()
	return spawningVal
}

// setSpawning raises or lowers the bracket in tmux and in the cache.
func setSpawning(on bool) {
	v := "0"
	if on {
		v = "1"
	}
	spawningMu.Lock()
	spawningVal, spawningAt = on, time.Now()
	spawningMu.Unlock()
	tmuxCmd("set-option", "-g", "@tabby_spawning", v).Run()
}
