//go:build mage

// Loadbars mage targets: build, test, install (same style as hexai).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const binaryName = "loadbars"

// Default target: build.
func Default() { mg.Deps(Build) }

// Build compiles the loadbars binary.
func Build() error {
	return sh.RunV("go", "build", "-o", binaryName, "./cmd/loadbars")
}

// Test runs the test suite.
func Test() error {
	return sh.RunV("go", "test", "./...")
}

// Install copies the built binary to GOPATH/bin (defaults to ~/go/bin when GOPATH is unset).
func Install() error {
	mg.Deps(Build)
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
		gopath = filepath.Join(home, "go")
	}
	bin := filepath.Join(gopath, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", bin, err)
	}
	return sh.RunV("cp", "-v", binaryName, filepath.Join(bin, binaryName))
}
