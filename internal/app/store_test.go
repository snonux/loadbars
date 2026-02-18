package app

import (
	"testing"

	"codeberg.org/snonux/loadbars/internal/collector"
	"codeberg.org/snonux/loadbars/internal/stats"
)

func TestSetDisk(t *testing.T) {
	s := NewStore()
	disk := collector.DiskLine{
		Device:       "sda",
		SectorsRead:  1000,
		SectorsWrite: 2000,
		IoTicks:      50,
	}
	s.SetDisk("host1", "sda", disk, 1.0)

	snap := s.Snapshot()
	h := snap["host1"]
	if h == nil {
		t.Fatal("expected host1 in snapshot")
	}
	ds, ok := h.Disk["sda"]
	if !ok {
		t.Fatal("expected sda in Disk map")
	}
	if ds.SectorsRead != 1000 || ds.SectorsWrite != 2000 || ds.IoTicks != 50 || ds.Stamp != 1.0 {
		t.Errorf("disk stamp mismatch: got %+v", ds)
	}
}

func TestSnapshotDiskDeepCopy(t *testing.T) {
	s := NewStore()
	disk := collector.DiskLine{
		Device:       "sda",
		SectorsRead:  100,
		SectorsWrite: 200,
	}
	s.SetDisk("host1", "sda", disk, 1.0)

	snap1 := s.Snapshot()

	// Mutate the store after snapshot
	disk2 := collector.DiskLine{
		Device:       "sda",
		SectorsRead:  999,
		SectorsWrite: 888,
	}
	s.SetDisk("host1", "sda", disk2, 2.0)

	// snap1 should be unchanged (deep copy)
	ds := snap1["host1"].Disk["sda"]
	if ds.SectorsRead != 100 || ds.SectorsWrite != 200 {
		t.Errorf("snapshot was mutated: got sr=%d sw=%d, want sr=100 sw=200",
			ds.SectorsRead, ds.SectorsWrite)
	}

	// Verify new snapshot has updated values
	snap2 := s.Snapshot()
	ds2 := snap2["host1"].Disk["sda"]
	if ds2.SectorsRead != 999 || ds2.SectorsWrite != 888 {
		t.Errorf("snap2: got sr=%d sw=%d, want sr=999 sw=888",
			ds2.SectorsRead, ds2.SectorsWrite)
	}
}

// Ensure Store still satisfies the collector.StatsStore interface (which now includes SetDisk).
var _ collector.StatsStore = (*Store)(nil)
var _ stats.Source = (*Store)(nil)
