package collector

import _ "embed"

// RemoteScript is the loadbars-remote.sh script embedded for local and SSH execution.
// Path is relative to this file's directory (internal/collector).
//
//go:embed scriptdata/loadbars-remote.sh
var RemoteScript []byte
