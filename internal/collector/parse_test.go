package collector

import (
	"testing"
)

func TestParseCPULine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantName  string
		wantUser  int64
		wantTotal int64
		wantErr   bool
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
		name      string
		line      string
		wantKey   string
		wantValue int64
		wantOK    bool
	}{
		{"MemTotal", "MemTotal:       123456 kB", "MemTotal", 123456, true},
		{"MemFree", "MemFree:         99999 kB", "MemFree", 99999, true},
		{"Buffers_zero", "Buffers:             0 kB", "Buffers", 0, true},
		{"not_a_mem_line", "not a mem line", "", 0, false},
		{"empty_string", "", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseMemLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseMemLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Key != tt.wantKey || got.Value != tt.wantValue {
				t.Errorf("ParseMemLine(%q) = %+v, want key=%q value=%d", tt.line, got, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestParseNetLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantIface string
		wantB     int64
		wantTb    int64
		wantErr   bool
	}{
		{"simple", "eth0:b=1000;tb=2000;p=10;tp=20;e=0;te=0;d=0;td=0", "eth0", 1000, 2000, false},
		{"with_space", "eth0:b=100;tb=200 p=0;tp=0;e=0;te=0;d=0;td=0", "eth0", 100, 200, false},
		{"no_colon", "eth0 b=1", "", 0, 0, true},
		// Linux /proc/net/dev style as emitted by loadbars-remote.sh (iface rx_bytes rx_packets ... tx_bytes ...)
		{"linux_style", "eth0:b=123456789;tb=987654321;p=1000;tp=2000 e=0;te=0;d=0;td=0", "eth0", 123456789, 987654321, false},
		{"lo_interface", "lo:b=0;tb=0;p=0;tp=0 e=0;te=0;d=0;td=0", "lo", 0, 0, false},
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

func TestParseDiskLine(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantDevice   string
		wantSR       int64
		wantSW       int64
		wantRT       int64
		wantWT       int64
		wantIO       int64
		wantErr      bool
	}{
		{"full", "sda:rs=1000;ws=2000;rt=50;wt=100;io=120", "sda", 1000, 2000, 50, 100, 120, false},
		{"nvme", "nvme0n1:rs=500;ws=300;rt=10;wt=20;io=30", "nvme0n1", 500, 300, 10, 20, 30, false},
		{"zeros", "vda:rs=0;ws=0;rt=0;wt=0;io=0", "vda", 0, 0, 0, 0, 0, false},
		{"large_counters", "sda:rs=9999999999;ws=8888888888;rt=100;wt=200;io=300", "sda", 9999999999, 8888888888, 100, 200, 300, false},
		{"partial_fields", "sda:rs=100;ws=200", "sda", 100, 200, 0, 0, 0, false},
		{"no_colon", "sda rs=100", "", 0, 0, 0, 0, 0, true},
		{"empty", "", "", 0, 0, 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDiskLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDiskLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Device != tt.wantDevice {
				t.Errorf("Device = %q, want %q", got.Device, tt.wantDevice)
			}
			if got.SectorsRead != tt.wantSR || got.SectorsWrite != tt.wantSW {
				t.Errorf("Sectors = read:%d write:%d, want read:%d write:%d", got.SectorsRead, got.SectorsWrite, tt.wantSR, tt.wantSW)
			}
			if got.ReadTicks != tt.wantRT || got.WriteTicks != tt.wantWT || got.IoTicks != tt.wantIO {
				t.Errorf("Ticks = rt:%d wt:%d io:%d, want rt:%d wt:%d io:%d", got.ReadTicks, got.WriteTicks, got.IoTicks, tt.wantRT, tt.wantWT, tt.wantIO)
			}
		})
	}
}
