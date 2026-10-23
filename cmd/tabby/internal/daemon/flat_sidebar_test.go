package daemon

import (
	"strings"
	"testing"

	"github.com/brendandebeasi/tabby/pkg/config"
	"github.com/brendandebeasi/tabby/pkg/grouping"
	"github.com/brendandebeasi/tabby/pkg/tmux"
	"github.com/stretchr/testify/assert"
)

// Fork behaviour: sidebar.flat and sidebar.plain_names (both default on).

func flatCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	c := newRenderCoordinator(t)
	c.config.Sidebar.Flat = nil       // absent key => flat
	c.config.Sidebar.PlainNames = nil // absent key => plain names
	c.config.Sidebar.Header.Height = config.Ptr(0)
	return c
}

func TestFlat_DefaultGroupHasNoHeaderOrTree(t *testing.T) {
	c := flatCoordinator(t)
	c.stateMu.Lock()
	c.windows = []tmux.Window{testWindow("alpha", true, "zsh"), testWindow("beta", false, "zsh")}
	c.grouped = []grouping.GroupedWindows{{Name: "Default", Windows: c.windows}}
	c.windowVisualPos = map[string]int{"@alpha": 1, "@beta": 2}
	c.stateMu.Unlock()
	out := stripAnsi(c.RenderForClient("@alpha", 30, 24).Content)
	assert.NotContains(t, out, "Default", "the catch-all group must not be drawn as a header")
	assert.NotContains(t, out, "├", "ungrouped windows carry no tree glyphs")
	assert.NotContains(t, out, "└")
	assert.Contains(t, out, "1. alpha")
	assert.Contains(t, out, "2. beta")
}

func TestFlat_UserGroupKeepsHeader(t *testing.T) {
	c := flatCoordinator(t)
	c.stateMu.Lock()
	c.windows = []tmux.Window{testWindow("alpha", true, "zsh"), testWindow("work", false, "zsh")}
	c.grouped = []grouping.GroupedWindows{
		{Name: "Default", Windows: c.windows[:1]},
		{Name: "pangram", Theme: config.Theme{Bg: "#3498db"}, Windows: c.windows[1:]},
	}
	c.windowVisualPos = map[string]int{"@alpha": 1, "@work": 2}
	c.stateMu.Unlock()
	out := stripAnsi(c.RenderForClient("@alpha", 30, 24).Content)
	assert.Contains(t, out, "pangram", "a group the user made keeps its header")
	assert.NotContains(t, out, "Default")
	// The grouped window sits under its header with a tree glyph; the
	// ungrouped one does not.
	lines := strings.Split(out, "\n")
	var alphaLine, workLine string
	for _, l := range lines {
		if strings.Contains(l, "alpha") {
			alphaLine = l
		}
		if strings.Contains(l, "work") {
			workLine = l
		}
	}
	assert.NotContains(t, alphaLine, "└")
	assert.Contains(t, workLine, "└")
}

func TestPlainNames_ProgramThenDirectory(t *testing.T) {
	c := flatCoordinator(t)
	shell := testWindow("azul-frontend", true, "zsh")
	assert.Equal(t, "azul-frontend", c.composeTabBaseName(shell), "a shell shows the directory, not a code")
	prog := testWindow("workspaces", true, "htop")
	assert.Equal(t, "htop", c.composeTabBaseName(prog), "a running program wins over the directory")
	ai := testWindow("tabby", true, "zsh")
	ai.AITitle = "refactor auth"
	assert.Equal(t, "refactor auth", c.composeTabBaseName(ai), "an AI summary wins and carries no code prefix")
}
