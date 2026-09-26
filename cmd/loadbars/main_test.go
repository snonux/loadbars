package main

import (
	"io"
	"reflect"
	"testing"

	"github.com/snonux/loadbars/internal/config"
	"github.com/snonux/loadbars/internal/constants"
)

func TestParseArgs_hostsMixedWithFlags(t *testing.T) {
	cfg := config.Default()
	_, err := parseArgs([]string{"web1", "root@web2", "--showmem", "--hosts", "db1,alice@db2", "web3", "--showcores"}, &cfg, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	want := []string{"db1", "db2:alice", "web1", "web2:root", "web3"}
	if !reflect.DeepEqual(cfg.Hosts, want) {
		t.Errorf("Hosts = %v, want %v", cfg.Hosts, want)
	}
	if !cfg.ShowMem {
		t.Error("--showmem after positional hosts was ignored")
	}
	if cfg.CPUMode != constants.CPUModeCores {
		t.Errorf("--showcores: CPUMode = %d, want %d", cfg.CPUMode, constants.CPUModeCores)
	}
}

func TestParseArgs_flagsOverrideConfigFile(t *testing.T) {
	cfg := config.Default()
	// Simulate values loaded from ~/.loadbarsrc.
	cfg.ShowMem = true
	cfg.Height = 300
	cfg.NetLink = "10gbit"
	_, err := parseArgs([]string{"--showmem=false", "--height", "200"}, &cfg, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.ShowMem {
		t.Error("--showmem=false did not override the config file")
	}
	if cfg.Height != 200 {
		t.Errorf("Height = %d, want 200", cfg.Height)
	}
	if cfg.NetLink != "10gbit" {
		t.Errorf("NetLink = %q, want config file value 10gbit", cfg.NetLink)
	}
}

func TestParseArgs_allConfigKeysHaveFlags(t *testing.T) {
	cfg := config.Default()
	args := []string{
		"--showload", "--showavgline", "--showioavgline", "--showseparators",
		"--diskaverage", "3", "--diskmode", "1", "--loadmax", "8", "--diskmax", "1000",
	}
	if _, err := parseArgs(args, &cfg, io.Discard); err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !cfg.ShowLoad || !cfg.ShowAvgLine || !cfg.ShowIOAvgLine || !cfg.ShowSeparators {
		t.Errorf("display flags not set: %+v", cfg)
	}
	if cfg.DiskAverage != 3 || cfg.DiskMode != 1 || cfg.LoadMax != 8 || cfg.DiskMax != 1000 {
		t.Errorf("numeric flags not set: %+v", cfg)
	}
}

func TestParseArgs_doubleDashEndsFlags(t *testing.T) {
	cfg := config.Default()
	if _, err := parseArgs([]string{"--showmem", "--", "-odd-host"}, &cfg, io.Discard); err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !reflect.DeepEqual(cfg.Hosts, []string{"-odd-host"}) {
		t.Errorf("Hosts = %v", cfg.Hosts)
	}
}

func TestParseArgs_rejectsInvalidValues(t *testing.T) {
	for _, args := range [][]string{
		{"--cpumode", "3"},
		{"--diskmode", "-1"},
		{"--cpuaverage", "0"},
		{"--loadmax", "-2"},
		{"--nosuchflag"},
	} {
		cfg := config.Default()
		if _, err := parseArgs(args, &cfg, io.Discard); err == nil {
			t.Errorf("parseArgs(%v): expected error", args)
		}
	}
}

func TestParseArgs_versionAndHelp(t *testing.T) {
	cfg := config.Default()
	opts, err := parseArgs([]string{"--version"}, &cfg, io.Discard)
	if err != nil || !opts.showVer {
		t.Errorf("--version: opts=%+v err=%v", opts, err)
	}
	opts, err = parseArgs([]string{"--help"}, &cfg, io.Discard)
	if err != nil || !opts.showHelp {
		t.Errorf("--help: opts=%+v err=%v", opts, err)
	}
}
