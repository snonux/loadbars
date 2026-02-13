//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	binaryName = "loadbars"
	shareName  = "loadbars"
)

// Default sets the default target (build).
func Default() { mg.Deps(Build) }

// Build compiles the loadbars binary.
func Build() error {
	return sh.RunV("go", "build", "-o", binaryName, "./cmd/loadbars")
}

// Test runs the test suite.
func Test() error {
	return sh.RunV("go", "test", "./...")
}

// Install builds and installs the binary, script, and NOTICE under DESTDIR.
// Usage: DESTDIR=/tmp/install mage install  (or  mage install  for DESTDIR empty)
func Install() error {
	mg.Deps(Build)
	dest := getDestDir()
	binDir := filepath.Join(dest, "usr", "bin")
	shareDir := filepath.Join(dest, "usr", "share", shareName)
	scriptsDir := filepath.Join(shareDir, "scripts")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", binDir, err)
	}
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", scriptsDir, err)
	}
	if err := sh.Copy(filepath.Join(binDir, binaryName), binaryName); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := sh.Copy(filepath.Join(scriptsDir, "loadbars-remote.sh"), "scripts/loadbars-remote.sh"); err != nil {
		return fmt.Errorf("copy script: %w", err)
	}
	if err := sh.Copy(filepath.Join(shareDir, "NOTICE"), "NOTICE"); err != nil {
		return fmt.Errorf("copy NOTICE: %w", err)
	}
	fmt.Printf("Installed to %s (binary: %s/usr/bin/%s)\n", dest, dest, binaryName)
	return nil
}

// Uninstall removes files installed by Install from DESTDIR.
func Uninstall() error {
	return deinstall()
}

// Deinstall is an alias for Uninstall.
func Deinstall() error {
	return deinstall()
}

func deinstall() error {
	dest := getDestDir()
	if dest == "" {
		return fmt.Errorf("DESTDIR must be set to uninstall (e.g. DESTDIR=/tmp/install mage uninstall)")
	}
	paths := []string{
		filepath.Join(dest, "usr", "bin", binaryName),
		filepath.Join(dest, "usr", "share", shareName, "scripts", "loadbars-remote.sh"),
		filepath.Join(dest, "usr", "share", shareName, "NOTICE"),
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	// Remove empty dirs
	_ = os.Remove(filepath.Join(dest, "usr", "share", shareName, "scripts"))
	_ = os.Remove(filepath.Join(dest, "usr", "share", shareName))
	fmt.Printf("Uninstalled from %s\n", dest)
	return nil
}

func getDestDir() string {
	if d := os.Getenv("DESTDIR"); d != "" {
		return filepath.Clean(d)
	}
	return ""
}
