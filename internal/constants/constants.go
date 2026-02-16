package constants

// Paths and limits
const (
	ConfFile            = ".loadbarsrc"
	CSSHConfFile        = "/etc/clusters"
	CSSHMaxRecursion    = 10
	ColorDepth          = 32
	Interval            = 0.14 // CPU sampling interval (seconds)
	IntervalNet         = 3.0  // Network stats interval
	IntervalMem         = 1.0  // Memory stats interval
	IntervalSDL         = 0.14 // Display refresh interval
	IntervalSDLWarn     = 1.0  // Warn if loop is slower than this
	SystemBlueThreshold = 30   // CPU system % for lighter blue
	UserOrangeThreshold = 70   // CPU user % for orange
	UserYellowThreshold = 50   // CPU user % for dark yellow
)

// Exit codes
const (
	Success  = 0
	EUnknown = 1
	ENoHost  = 2
)

// Copyright string
const Copyright = "2010-2026 (c) Paul Buetow <loadbars@dev.buetow.org>"

// RGB holds R, G, B in 0-255 for SDL.
type RGB struct{ R, G, B uint8 }

// Colors used by the display (matching Perl Constants.pm)
var (
	Black      = RGB{0x00, 0x00, 0x00}
	Blue0      = RGB{0x00, 0x00, 0xff}
	LightBlue  = RGB{0x00, 0x00, 0xdd}
	LightBlue0 = RGB{0x00, 0x00, 0xcc}
	Blue       = RGB{0x00, 0x00, 0x88}
	Green      = RGB{0x00, 0x90, 0x00}
	LimeGreen  = RGB{0x50, 0xc8, 0x00}
	LightGreen = RGB{0x00, 0xf0, 0x00}
	Orange     = RGB{0xff, 0x70, 0x00}
	Purple     = RGB{0xa0, 0x20, 0xf0}
	Pink       = RGB{0xff, 0x40, 0xff}
	Red        = RGB{0xff, 0x00, 0x00}
	White      = RGB{0xff, 0xff, 0xff}
	Grey0      = RGB{0x11, 0x11, 0x11}
	Grey       = RGB{0xaa, 0xaa, 0xaa}
	DarkGrey   = RGB{0x15, 0x15, 0x15}
	Yellow0    = RGB{0xff, 0xa0, 0x00}
	Yellow     = RGB{0xff, 0xc0, 0x00}
)

// BytesPerSec for link speed reference (bytes per second at given mbit)
var (
	BytesMbit    = 125000
	Bytes10Mbit  = 1250000
	Bytes100Mbit = 12500000
	BytesGbit    = 125000000
	Bytes10Gbit  = 1250000000
)
