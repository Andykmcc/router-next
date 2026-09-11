package main

import (
	"fmt"
	"os"
	"time"

	"router/pkg/gtfs"
	"router/pkg/raptor"
	"testing"
)

func assertSnapshotMatches(t *testing.T, rt *raptor.RaptorTable, snapshotId string) {
	t.Helper()

	snapshot := rt.SnapshotString()
	fileName := fmt.Sprintf("./snapshots/%s.txt", snapshotId)

	if os.Getenv("RAPTOR_UPDATE_SNAPSHOTS") != "" {
		if err := os.WriteFile(fileName, []byte(snapshot), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", snapshotId, err)
		}

		t.Logf("%s.txt regenerated", snapshotId)

		return
	}

	bytes, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("read snapshot %s.txt: %v (run with RAPTOR_UPDATE_SNAPSHOTS=1 to create it)", snapshotId, err)
	}

	if string(bytes) != snapshot {
		t.Fatalf("%s.txt snapshot mismatch; if intended, re-run with RAPTOR_UPDATE_SNAPSHOTS=1 to regenerate", snapshotId)
	}
}

func TestRaptorBuild(t *testing.T) {
	gtfsTable, err := gtfs.ParseGtfs("./testdata/gtfs_04162026.zip")
	if err != nil {
		t.Fatalf("GTFS parsing failed: %v", err)
	}

	d1 := time.Date(2026, time.April, 9, 0, 0, 0, 0, time.UTC)
	raptorTable1, err := raptor.BuildRaptorTable(gtfsTable, gtfs.TimeToGTFSDate(d1))
	if err != nil {
		t.Fatalf("Raptor Table generation failed: %v", err)
	}

	assertSnapshotMatches(t, raptorTable1, d1.Format(time.DateOnly))

	d2 := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)
	raptorTable2, err := raptor.BuildRaptorTable(gtfsTable, gtfs.TimeToGTFSDate(d2))
	if err != nil {
		t.Fatalf("Raptor Table generation failed: %v", err)
	}

	assertSnapshotMatches(t, raptorTable2, d2.Format((time.DateOnly)))
}
