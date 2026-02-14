// +build darwin

package display

import (
	"os"
	"os/exec"
	"time"
)

// activateWindow brings the SDL window to the foreground on macOS
func activateWindow() {
	// Give SDL a moment to create the window
	go func() {
		time.Sleep(500 * time.Millisecond)
		// Get the executable path
		execPath, err := os.Executable()
		if err != nil {
			return
		}
		// Use open -a to bring the window to foreground
		exec.Command("open", "-a", execPath).Run()
	}()
}
