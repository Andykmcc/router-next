package raptor

import (
	"slices"
	"testing"

	"router/pkg/gtfs"
	"router/pkg/types"
)

// TestIterateOverRaptorRoutesTripIndexSpace guards against a regression where
// RaptorTripID values - indices into the date-filtered trip list built by
// enumerateGtfsTrips(gtfsTable.TripsForDate(date)) - were used to index the
// FULL, differently-ordered gtfsTable.Trips slice instead. Because the two
// slices disagree at almost every position on a real feed, every RaptorRoute
// ended up labelled with an essentially unrelated route's metadata.
func TestIterateOverRaptorRoutesTripIndexSpace(t *testing.T) {
	other := gtfs.GTFSTrip{GtfsId: "trip-other", GtfsRouteId: "OTHER", GtfsServiceId: "s", Headsign: ""}
	target := gtfs.GTFSTrip{GtfsId: "trip-target", GtfsRouteId: "TARGET", GtfsServiceId: "s", Headsign: ""}

	// fullTrips stands in for gtfsTable.Trips: "other" happens to sort first,
	// purely a file-order artifact unrelated to which trips run today.
	fullTrips := []gtfs.GTFSTrip{other, target}

	// activeTrips stands in for gtfsTable.TripsForDate(date): only "target"
	// runs today, so it lands at index 0 - a DIFFERENT position than in
	// fullTrips. This positional disagreement is the whole bug.
	activeTrips := []gtfs.GTFSTrip{target}

	stopIdMap := map[gtfs.GTFSStopID]types.StopID{"s0": 0, "s1": 1}
	tripIdMap := enumerateGtfsTrips(activeTrips)

	stopTimes := []gtfs.GTFSStopTime{
		{GtfsTripId: "trip-target", GtfsStopId: "s0", ArrivalTime: 0, DepartureTime: 0, StopSequence: 0},
		{GtfsTripId: "trip-target", GtfsStopId: "s1", ArrivalTime: 60, DepartureTime: 60, StopSequence: 1},
	}

	raptorTrips := groupRaptorTrips(stopTimes, stopIdMap, tripIdMap)
	raptorRoutes := groupRaptorRoutes(raptorTrips)

	routeMap := map[gtfs.GTFSRouteID]*gtfs.GTFSRoute{
		"OTHER":  {GtfsId: "OTHER", GtfsAgencyId: "a", ShortName: "O", LongName: "Other", RouteType: 3, Color: ""},
		"TARGET": {GtfsId: "TARGET", GtfsAgencyId: "a", ShortName: "T", LongName: "Target", RouteType: 3, Color: ""},
	}

	// Correct usage: iterateOverRaptorRoutes must be given the SAME slice
	// (index-space) that produced the RaptorTripIDs baked into raptorRoutes.
	routes, _, _, _, _, _, _, _, _ := iterateOverRaptorRoutes(raptorRoutes, routeMap, activeTrips, 2)
	if len(routes) != 1 || routes[0].GtfsId != "TARGET" {
		t.Fatalf("with the correctly-scoped slice: routes = %+v, want a single route with GtfsId %q", routes, "TARGET")
	}

	// Scenario sanity check: passing the WRONG (full, unfiltered) slice - the
	// historical bug at pkg/raptor/build.go's call site - must reproduce the
	// mislabelling. If this stops failing, the scenario is no longer a valid
	// trap and needs to be reconstructed.
	buggyRoutes, _, _, _, _, _, _, _, _ := iterateOverRaptorRoutes(raptorRoutes, routeMap, fullTrips, 2)
	if len(buggyRoutes) != 1 || buggyRoutes[0].GtfsId != "OTHER" {
		t.Fatalf("scenario sanity check: passing the full slice should reproduce the historical bug (GtfsId %q), got %+v",
			"OTHER", buggyRoutes)
	}
}

func TestSortRoutesOrdersTripsByDeparture(t *testing.T) { // G regression
	routes := sortRoutes(map[string]*RaptorRoute{
		"0,1": {
			stopSequence: []types.StopID{0, 1},
			trips: []RaptorTrip{
				{{tripId: 0, departureTime: 30000}}, // late, listed first
				{{tripId: 1, departureTime: 29000}}, // early
			},
		},
	})

	if len(routes) != 1 || len(routes[0].trips) != 2 {
		t.Fatalf("routes = %+v", routes)
	}

	if routes[0].trips[0][0].departureTime != 29000 || routes[0].trips[1][0].departureTime != 30000 {
		t.Fatalf("trips not ordered by departure: %d then %d",
			routes[0].trips[0][0].departureTime, routes[0].trips[1][0].departureTime)
	}
}

func TestGroupRouteSegments(t *testing.T) { // regression: wrong StopIndex/RouteId in segments
	stopIdMap := map[gtfs.GTFSStopID]types.StopID{"s0": 0, "s1": 1, "s2": 2}
	tripIdMap := enumerateGtfsTrips([]gtfs.GTFSTrip{
		{GtfsId: "t0", GtfsRouteId: "R0", GtfsServiceId: "s"},
		{GtfsId: "t1", GtfsRouteId: "R1", GtfsServiceId: "s"},
	})
	stopTimes := []gtfs.GTFSStopTime{
		{GtfsTripId: "t0", GtfsStopId: "s0", ArrivalTime: 0, DepartureTime: 0, StopSequence: 0},
		{GtfsTripId: "t0", GtfsStopId: "s1", ArrivalTime: 60, DepartureTime: 60, StopSequence: 1},
		{GtfsTripId: "t1", GtfsStopId: "s1", ArrivalTime: 0, DepartureTime: 0, StopSequence: 0},
		{GtfsTripId: "t1", GtfsStopId: "s2", ArrivalTime: 60, DepartureTime: 60, StopSequence: 1},
	}

	raptorRoutes := groupRaptorRoutes(groupRaptorTrips(stopTimes, stopIdMap, tripIdMap))
	// route 0 serves stops [0,1], route 1 serves [1,2]
	segments, offsets := groupRouteSegments(raptorRoutes, []uint32{1, 2, 1}, 3)

	wantOffsets := StopRouteSegmentOffsets{0, 1, 3, 4}
	if !slices.Equal(offsets, wantOffsets) {
		t.Fatalf("offsets = %v, want %v", offsets, wantOffsets)
	}

	want := []RouteSegment{
		{RouteId: 0, StopIndex: 0}, // stop 0
		{RouteId: 0, StopIndex: 1}, // stop 1
		{RouteId: 1, StopIndex: 0},
		{RouteId: 1, StopIndex: 1}, // stop 2
	}

	for i, seg := range segments {
		if seg != want[i] {
			t.Fatalf("segments[%d] = %+v, want %+v", i, seg, want[i])
		}
	}
}

func TestExtractSelfTransfers(t *testing.T) { // regression: type filter / same-stop filter
	stopIdMap := map[gtfs.GTFSStopID]types.StopID{"a": 0, "b": 1}
	transfers := []gtfs.GTFSTransfer{
		{FromStopId: "a", ToStopId: "a", TransferType: 2, MinTransferTime: 30},  // kept
		{FromStopId: "b", ToStopId: "b", TransferType: 0, MinTransferTime: 99}, // wrong type: ignored
		{FromStopId: "a", ToStopId: "b", TransferType: 2, MinTransferTime: 5},  // not self: ignored
	}

	got := extractSelfTransfers(transfers, stopIdMap)

	if len(got) != 2 || got[0] != 30 || got[1] != 0 {
		t.Fatalf("minTransferTime = %v, want [30 0]", got)
	}
}
