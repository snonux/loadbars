package collector

import _ "embed"

// LinuxScript contains the embedded loadbars-remote.sh script for Linux hosts
//
//go:embed loadbars-remote.sh
var LinuxScript []byte

// DarwinScript contains the embedded loadbars-remote-darwin.sh script for macOS hosts
//
//go:embed loadbars-remote-darwin.sh
var DarwinScript []byte
