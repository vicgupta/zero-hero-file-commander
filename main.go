package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	appName    = "Zero Hero File Commander"
	appVersion = "0.1.0"
)

// cliFlags holds every command-line flag's destination.
type cliFlags struct {
	left, right, theme             string
	hidden, brief, viKeys, version bool
}

// parseFlags builds a fresh FlagSet per call (rather than using the global
// flag.CommandLine) so this is safely callable more than once — from main,
// and repeatedly from tests without "flag redefined" panics. ContinueOnError
// means a bad flag is reported via the returned error instead of exiting the
// process, which also makes it testable.
func parseFlags(args []string) (*flag.FlagSet, cliFlags, error) {
	var f cliFlags
	fs := flag.NewFlagSet("zhfc", flag.ContinueOnError)
	fs.StringVar(&f.left, "left", "", "initial directory for the left panel")
	fs.StringVar(&f.right, "right", "", "initial directory for the right panel")
	fs.StringVar(&f.theme, "theme", "", "color theme: norton, nightowl, or opencode")
	fs.BoolVar(&f.hidden, "hidden", false, "show hidden files")
	fs.BoolVar(&f.brief, "brief", false, "start panels in brief (name-only) display mode")
	fs.BoolVar(&f.viKeys, "vi-keys", false, "enable h/j/k/l/n/u/v navigation and fast search")
	fs.BoolVar(&f.version, "version", false, "print the version and exit")
	err := fs.Parse(args)
	return fs, f, err
}

// applyFlags layers the parsed flags onto a loaded config. It uses fs.Visit
// rather than reading the fields directly, so a bool flag that was never
// passed on the command line — and so holds its Go zero value — can never
// clobber a `true` the user had already saved in their config file.
func applyFlags(c cfg, fs *flag.FlagSet, f cliFlags) (cfg, error) {
	var err error
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "theme":
			if _, ok := themes[f.theme]; !ok {
				err = fmt.Errorf("unknown theme %q (available: %v)", f.theme, themeNames)
				return
			}
			c.Theme = f.theme
		case "hidden":
			c.Hidden = f.hidden
		case "brief":
			c.Brief = f.brief
		case "vi-keys":
			c.ViKeys = f.viKeys
		}
	})
	return c, err
}

// resolveDirs merges the -left/-right flags and positional args into the two
// starting panel paths. Whichever side is still unset falls back to wd — the
// shell's current directory — never a remembered one, so zhfc always opens
// where it was launched.
func resolveDirs(flagL, flagR string, args []string, wd string) (string, string) {
	startL, startR := flagL, flagR
	// Positional args: zhfc [dir] sets the left panel; zhfc [left] [right] sets both.
	if len(args) > 0 {
		startL = args[0]
		if len(args) > 1 {
			startR = args[1]
		}
	}
	if startL == "" {
		startL = wd
	}
	if startR == "" {
		startR = startL
	}
	return startL, startR
}

func main() {
	fs, f, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(2) // flag already printed the usage/error message
	}
	if f.version {
		fmt.Printf("zhfc %s — %s\n", appVersion, appName)
		return
	}

	c := loadConfig()
	c, err = applyFlags(c, fs, f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zhfc:", err)
		os.Exit(2)
	}
	wd, _ := os.Getwd()
	startL, startR := resolveDirs(f.left, f.right, fs.Args(), wd)

	m := newModel(startL, startR, c)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "zhfc:", err)
		os.Exit(1)
	}
}
