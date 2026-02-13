package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_parseReader(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantBar int
		wantExt bool
	}{
		{"empty", "", 1200, false},
		{"barwidth", "barwidth=42\n", 42, false},
		{"extended_1", "extended=1\n", 1200, true},
		{"extended_true", "extended=true\n", 1200, true},
		{"comments", "# foo\nbarwidth=10\n# bar\n", 10, false},
		{"unknown_key", "barwidth=5\nunknown=ignored\n", 5, false},
		{"multiple", "barwidth=30\nextended=1\nshowcores=1\n", 30, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			f, _ := os.Open(os.DevNull)
			defer f.Close()
			// Use a temp file with the content since parseReader takes *os.File
			dir := t.TempDir()
			path := filepath.Join(dir, "rc")
			if err := os.WriteFile(path, []byte(tt.input), 0600); err != nil {
				t.Fatal(err)
			}
			f2, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f2.Close()
			if err := c.parseReader(f2); err != nil {
				t.Fatal(err)
			}
			if c.BarWidth != tt.wantBar {
				t.Errorf("BarWidth = %d, want %d", c.BarWidth, tt.wantBar)
			}
			if c.Extended != tt.wantExt {
				t.Errorf("Extended = %v, want %v", c.Extended, tt.wantExt)
			}
		})
	}
}

func TestConfig_writeTo(t *testing.T) {
	c := Default()
	c.BarWidth = 25
	c.ShowCores = true
	dir := t.TempDir()
	path := filepath.Join(dir, "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = c.writeTo(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(data) == 0 {
		t.Error("writeTo wrote nothing")
	}
	if !bytes.Contains(data, []byte("barwidth=25")) {
		t.Errorf("expected barwidth=25 in %s", data)
	}
	if !bytes.Contains(data, []byte("showcores=1")) {
		t.Errorf("expected showcores=1 in %s", data)
	}
}

func TestGetClusterHostsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters")

	tests := []struct {
		name      string
		content   string
		cluster   string
		wantHosts []string
		wantErr   bool
	}{
		{"single_host", "foo host1\n", "foo", []string{"host1"}, false},
		{"two_hosts", "bar host1 host2\n", "bar", []string{"host1", "host2"}, false},
		{"missing_returns_cluster", "x y\n", "missing", []string{"missing"}, false},
		{"recursive", "a b\nb c\nc d\n", "a", []string{"d"}, false},
		{"cycle", "a b\nb a\n", "a", nil, true},
		{"comment_ignored", "# comment\na h1\n", "a", []string{"h1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := GetClusterHostsFromFile(tt.cluster, path)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetClusterHostsFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.wantHosts) {
				t.Errorf("got %v, want %v", got, tt.wantHosts)
				return
			}
			for i := range got {
				if got[i] != tt.wantHosts[i] {
					t.Errorf("got[%d] = %s, want %s", i, got[i], tt.wantHosts[i])
				}
			}
		})
	}
}
