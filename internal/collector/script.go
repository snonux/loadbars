package collector

import _ "embed"

// LinuxScript contains the embedded loadbars-remote.sh script for Linux hosts
//
//go:embed loadbars-remote.sh
var LinuxScript []byte
