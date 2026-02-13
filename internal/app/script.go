package app

import (
	"os"
	"path/filepath"
)

// ScriptPath returns the path to the loadbars-remote.sh script.
// It looks for scripts/loadbars-remote.sh relative to the executable, then current dir.
func ScriptPath() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		// When installed: exe is /usr/bin/loadbars, script might be /usr/share/loadbars/scripts/
		for _, sub := range []string{"scripts/loadbars-remote.sh", "../scripts/loadbars-remote.sh", "../../scripts/loadbars-remote.sh"} {
			p := filepath.Join(dir, sub)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	// Development: run from repo root
	if p, err := filepath.Abs("scripts/loadbars-remote.sh"); err == nil {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "scripts/loadbars-remote.sh"
}
