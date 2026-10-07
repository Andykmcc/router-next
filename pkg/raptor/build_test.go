package raptor_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"router/pkg/gtfs"
	"router/pkg/raptor"
	"router/pkg/types"
)

const testDate gtfs.GTFSDate = "20260409"

// buildTestTable writes a minimal GTFS feed to a zip in t.TempDir() and builds
// a RAPTOR table from it for testDate. The trips, stop_times and transfers
// arguments are CSV rows without headers.
//
// UNUSED sits at index 0 of stops.txt and no trip serves it, so a stop time
// that silently falls back to StopID 0 shows up in a route's stop sequence.
func buildTestTable(t *testing.T, trips, stopTimes, transfers string) *raptor.RaptorTable {
	t.Helper()

	files := map[string]string{
		"routes.txt": "route_id,agency_id,route_short_name,route_long_name,route_type,route_color\n" +
			"R1,AG,1,Line 1,3,FF0000\n",
		"trips.txt": "route_id,service_id,trip_id,trip_headsign\n" + trips,
		"calendar.txt": "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\n" +
			"WK,1,1,1,1,1,1,1,20260101,20261231\n",
		"calendar_dates.txt": "service_id,date,exception_type\n",
		"stops.txt": "stop_id,stop_name,stop_lat,stop_lon\n" +
			"UNUSED,Unused,37.70,-122.40\n" +
			"A,Stop A,37.71,-122.41\n" +
			"B,Stop B,37.72,-122.42\n" +
			"C,Stop C,37.73,-122.43\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\n" + stopTimes,
		"transfers.txt":  "from_stop_id,to_stop_id,transfer_type,min_transfer_time\n" + transfers,
	}

	zipPath := filepath.Join(t.TempDir(), "gtfs.zip")

	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	for name, content := range files {
		fileWriter, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("create %s in zip: %v", name, err)
		}

		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write %s to zip: %v", name, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	if err := zipFile.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	gtfsTable, err := gtfs.ParseGtfs(zipPath)
	if err != nil {
		t.Fatalf("ParseGtfs: %v", err)
	}

	rt, err := raptor.BuildRaptorTable(gtfsTable, testDate)
	if err != nil {
		t.Fatalf("BuildRaptorTable: %v", err)
	}

	return rt
}

func TestBuildRaptorTableSkipsUnknownStopIds(t *testing.T) {
	rt := buildTestTable(t,
		"R1,WK,T1,Downtown\n",
		"T1,08:00:00,08:00:00,A,1\n"+
			"T1,08:05:00,08:05:00,GHOST,2\n"+
			"T1,08:10:00,08:10:00,B,3\n"+
			"T1,08:15:00,08:15:00,C,4\n",
		"GHOST,GHOST,2,300\n"+
			"B,B,2,60\n",
	)

	if got := rt.NumRoutes(); got != 1 {
		t.Fatalf("NumRoutes() = %d, want 1", got)
	}

	wantStops := []types.StopID{1, 2, 3}
	if got := rt.StopsForRoute(0); !slices.Equal(got, wantStops) {
		t.Errorf("StopsForRoute(0) = %v, want %v (unknown stop_id must be skipped, not mapped to stop 0)", got, wantStops)
	}

	wantEvents := []raptor.StopEvent{
		{ArrivalTime: 8 * 3600, DepartureTime: 8 * 3600},
		{ArrivalTime: 8*3600 + 10*60, DepartureTime: 8*3600 + 10*60},
		{ArrivalTime: 8*3600 + 15*60, DepartureTime: 8*3600 + 15*60},
	}
	if got := rt.StopEventsForTrip(0, 0); !slices.Equal(got, wantEvents) {
		t.Errorf("StopEventsForTrip(0, 0) = %v, want %v", got, wantEvents)
	}

	if got := rt.MinTransferTime[0]; got != 0 {
		t.Errorf("MinTransferTime[0] = %d, want 0 (transfer for unknown stop_id must be skipped)", got)
	}

	if got := rt.MinTransferTime[2]; got != 60 {
		t.Errorf("MinTransferTime[2] = %d, want 60 (transfer for known stop B must be kept)", got)
	}
}

func TestBuildRaptorTableDropsTripWithOnlyUnknownStops(t *testing.T) {
	rt := buildTestTable(t,
		"R1,WK,T1,Downtown\n"+
			"R1,WK,T2,Nowhere\n",
		"T1,08:00:00,08:00:00,A,1\n"+
			"T1,08:10:00,08:10:00,B,2\n"+
			"T2,09:00:00,09:00:00,GHOST,1\n"+
			"T2,09:10:00,09:10:00,GHOST2,2\n",
		"",
	)

	if got := rt.NumRoutes(); got != 1 {
		t.Fatalf("NumRoutes() = %d, want 1 (a trip with no known stops must not form a route)", got)
	}

	wantStops := []types.StopID{1, 2}
	if got := rt.StopsForRoute(0); !slices.Equal(got, wantStops) {
		t.Errorf("StopsForRoute(0) = %v, want %v", got, wantStops)
	}

	if got := rt.NumTripsInRoute[0]; got != 1 {
		t.Errorf("NumTripsInRoute[0] = %d, want 1", got)
	}
}
