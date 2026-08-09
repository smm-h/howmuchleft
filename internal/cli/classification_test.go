package cli

import (
	"sort"
	"testing"

	"github.com/smm-h/strictcli/go/strictcli"
	"github.com/smm-h/stricttest/go/hygiene"
)

// classification pins every command's strictcli effect classification and its
// `consequential` declaration. The table is the specification: changing a row
// here is the deliberate edit that a reclassification requires, and adding a
// command without adding its row fails the test.
//
// Reasoning, per the strictcli effects contract (§1: `read_only` means no
// user-visible or consequential mutation; §8.1: `consequential` means the act
// is worth interrupting someone for).
//
// Every command is `mutating`, and the reason is shared rather than
// per-command: every handler opens with runMigrations(), which converts a
// legacy ~/.config/howmuchleft.json into TOML and renames the original to
// .bak, then writes any missing settings into
// ~/.config/howmuchleft/config.toml. Those writes happen on `version` and
// `colors` exactly as they happen on `profile install`, so no command here can
// honestly claim read_only, however little of its own work it does.
//
// Per-command, on top of that shared write:
//
//   - version, colors, config -- print only.
//   - demo -- animates on the terminal only.
//   - profile.list -- reads every discovered profile's usage (network reads
//     through the usage API and cache writes under the Claude dir).
//   - profile.install / profile.uninstall -- edit a Claude Code profile's
//     settings to add or remove the statusline hook.
//
// Nothing here is consequential. The bar is: destructive on a remote, creates
// a named external resource a rerun cannot un-create, or makes something
// public or live that was not before. The most invasive commands in this app,
// profile install and uninstall, edit one local settings file and are each
// other's exact undo.
var classification = map[string]struct {
	effect        string
	consequential bool
}{
	"version":           {strictcli.EffectMutating, false},
	"colors":            {strictcli.EffectMutating, false},
	"config":            {strictcli.EffectMutating, false},
	"demo":              {strictcli.EffectMutating, false},
	"profile.install":   {strictcli.EffectMutating, false},
	"profile.uninstall": {strictcli.EffectMutating, false},
	"profile.list":      {strictcli.EffectMutating, false},
}

// collectCommands flattens the app's command tree into dotted paths.
func collectCommands(app *strictcli.App) map[string]*strictcli.Command {
	out := map[string]*strictcli.Command{}
	for name, cmd := range app.Commands() {
		out[name] = cmd
	}
	var walk func(prefix string, g *strictcli.Group)
	walk = func(prefix string, g *strictcli.Group) {
		for name, cmd := range g.Commands {
			out[prefix+name] = cmd
		}
		for name, sub := range g.Groups {
			walk(prefix+name+".", sub)
		}
	}
	for name, g := range app.Groups() {
		walk(name+".", g)
	}
	return out
}

func TestCommandClassificationIsPinned(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))

	cmds := collectCommands(NewApp())

	var names []string
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		want, ok := classification[name]
		if !ok {
			t.Errorf("command %q is registered but has no pinned classification; add a row to classification with the reasoning", name)
			continue
		}
		if cmds[name].Effect != want.effect {
			t.Errorf("command %q: effect = %q, pinned %q", name, cmds[name].Effect, want.effect)
		}
		if cmds[name].Consequential != want.consequential {
			t.Errorf("command %q: consequential = %v, pinned %v", name, cmds[name].Consequential, want.consequential)
		}
	}
	for name := range classification {
		if _, ok := cmds[name]; !ok {
			t.Errorf("classification pins %q but no such command is registered", name)
		}
	}
}

// TestNoReservedGlobalFlagNames guards the framework's reserved quartet at the
// app level. Command-level flags are covered implicitly: strictcli panics at
// registration for a reserved name anywhere, and NewApp() registers everything.
func TestNoReservedGlobalFlagNames(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))

	reserved := map[string]bool{"dry-run": true, "yes": true, "quiet": true, "verbose": true}
	for _, f := range NewApp().GlobalFlags() {
		if reserved[f.Name] {
			t.Errorf("global flag %q is reserved by the framework", f.Name)
		}
	}
}
