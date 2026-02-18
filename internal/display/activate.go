//go:build !darwin

package display

// activateWindow is a no-op on non-macOS platforms
func activateWindow() {
	// Nothing needed on Linux/other platforms
}
