package collector

import (
	"testing"
)

func TestParseCPULine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantName string
		wantUser int64
		wantTotal int64
		wantErr bool
	}{
		{"normal", "cpu 100 0 50 200 0 0 0 0 0 0", "cpu", 100, 350, false},
		{"cpu0", "cpu0 10 0 5 80 0 0 0 0 0 0", "cpu0", 10, 95, false},
		{"short", "cpu 1 2 3", "cpu", 1, 6, false},
		{"empty", "", "", 0, 0, true},
		{"one_field", "cpu", "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCPULine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCPULine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.User != tt.wantUser {
				t.Errorf("User = %d, want %d", got.User, tt.wantUser)
			}
			if total := got.Total(); total != tt.wantTotal {
				t.Errorf("Total() = %d, want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestParseMemLine(t *testing.T) {
	tests := []struct {
		line      string
		wantKey   string
		wantValue int64
		wantOK    bool
	}{
		{"MemTotal:       123456 kB", "MemTotal", 123456, true},
		{"MemFree:         99999 kB", "MemFree", 99999, true},
		{"Buffers:             0 kB", "Buffers", 0, true},
		{"not a mem line", "", 0, false},
		{"", "", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParseMemLine(tt.line)
		if ok != tt.wantOK {
			t.Errorf("ParseMemLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !tt.wantOK {
			continue
		}
		if got.Key != tt.wantKey || got.Value != tt.wantValue {
			t.Errorf("ParseMemLine(%q) = %+v, want key=%q value=%d", tt.line, got, tt.wantKey, tt.wantValue)
		}
	}
}

func TestParseNetLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantIface string
		wantB   int64
		wantTb  int64
		wantErr bool
	}{
		{"simple", "eth0:b=1000;tb=2000;p=10;tp=20;e=0;te=0;d=0;td=0", "eth0", 1000, 2000, false},
		{"with_space", "eth0:b=100;tb=200 p=0;tp=0;e=0;te=0;d=0;td=0", "eth0", 100, 200, false},
		{"no_colon", "eth0 b=1", "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNetLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNetLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Iface != tt.wantIface || got.B != tt.wantB || got.Tb != tt.wantTb {
				t.Errorf("got %+v, want Iface=%s B=%d Tb=%d", got, tt.wantIface, tt.wantB, tt.wantTb)
			}
		})
	}
}

func TestParseLoadAvg(t *testing.T) {
	got := ParseLoadAvg("1.25;0.50;0.20")
	if got.Load1 != "1.25" || got.Load5 != "0.50" || got.Load15 != "0.20" {
		t.Errorf("ParseLoadAvg = %+v", got)
	}
	got2 := ParseLoadAvg("1.0")
	if got2.Load1 != "1.0" || got2.Load5 != "" || got2.Load15 != "" {
		t.Errorf("ParseLoadAvg(1.0) = %+v", got2)
	}
}
