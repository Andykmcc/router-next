package main

import (
	"cmp"
	"fmt"
	"slices"
	"testing"
	"time"

	"router/pkg/gtfs"
	"router/pkg/raptor"
	"router/pkg/types"
)

const maxReportedTripMismatches = 10

// TestRaptorTripsMatchGtfs checks that every trip stored in a RAPTOR route is the
// GTFS trip whose stop_times produced that route's stops and stop events, and
// that each RAPTOR route records the GTFS route of its first trip.
func TestRaptorTripsMatchGtfs(t *testing.T) {
	gtfsTable, err := gtfs.ParseGtfs("./testdata/gtfs_04162026.zip")
	if err != nil {
		t.Fatalf("GTFS parsing failed: %v", err)
	}

	date := time.Date(2026, time.April, 9, 0, 0, 0, 0, time.UTC)

	rt, err := raptor.BuildRaptorTable(gtfsTable, gtfs.TimeToGTFSDate(date))
	if err != nil {
		t.Fatalf("RAPTOR table generation failed: %v", err)
	}

	stopIdMap := make(map[gtfs.GTFSStopID]types.StopID, len(gtfsTable.Stops))
	for idx, stop := range gtfsTable.Stops {
		stopIdMap[stop.GtfsId] = types.StopID(idx)
	}

	stopTimesByTrip := groupStopTimesByTrip(gtfsTable.StopTimes)

	numTrips, numBadTrips, numBadRoutes := 0, 0, 0

	for routeId := range types.RouteID(rt.NumRoutes()) {
		routeIsBad := false

		for tripIdx := range rt.NumTripsInRoute[routeId] {
			numTrips++

			problems := tripMismatches(rt, routeId, tripIdx, stopTimesByTrip, stopIdMap)
			if len(problems) == 0 {
				continue
			}

			numBadTrips++
			routeIsBad = true

			if numBadTrips <= maxReportedTripMismatches {
				t.Errorf("route %d trip %d (%s): %v", routeId, tripIdx, rt.TripInRoute(routeId, tripIdx).GtfsId, problems)
			}
		}

		if routeIsBad {
			numBadRoutes++
		}
	}

	if numBadTrips > 0 {
		t.Errorf("%d of %d trips in %d of %d RAPTOR routes don't match their GTFS trip",
			numBadTrips, numTrips, numBadRoutes, rt.NumRoutes())
	}
}

func groupStopTimesByTrip(stopTimes []gtfs.GTFSStopTime) map[gtfs.GTFSTripID][]gtfs.GTFSStopTime {
	stopTimesByTrip := make(map[gtfs.GTFSTripID][]gtfs.GTFSStopTime)
	for _, stopTime := range stopTimes {
		stopTimesByTrip[stopTime.GtfsTripId] = append(stopTimesByTrip[stopTime.GtfsTripId], stopTime)
	}

	for _, tripStopTimes := range stopTimesByTrip {
		slices.SortFunc(tripStopTimes, func(stopTimeA, stopTimeB gtfs.GTFSStopTime) int {
			return cmp.Compare(stopTimeA.StopSequence, stopTimeB.StopSequence)
		})
	}

	return stopTimesByTrip
}

func tripMismatches(
	rt *raptor.RaptorTable,
	routeId types.RouteID,
	tripIdx uint32,
	stopTimesByTrip map[gtfs.GTFSTripID][]gtfs.GTFSStopTime,
	stopIdMap map[gtfs.GTFSStopID]types.StopID,
) []string {
	var problems []string

	trip := rt.TripInRoute(routeId, tripIdx)

	// A RAPTOR route groups trips by stop sequence, so it can hold trips of several
	// GTFS routes (e.g. a line and its express variant). Routes[r] is the GTFS route
	// of the first trip, which the stop_times checks below pin to the right trip.
	if tripIdx == 0 && trip.GtfsRouteId != rt.Routes[routeId].GtfsId {
		problems = append(problems,
			fmt.Sprintf("trip route %q != route %q", trip.GtfsRouteId, rt.Routes[routeId].GtfsId))
	}

	stopTimes := stopTimesByTrip[trip.GtfsId]
	stops := make([]types.StopID, 0, len(stopTimes))
	stopEvents := make([]raptor.StopEvent, 0, len(stopTimes))

	for _, stopTime := range stopTimes {
		// The build skips stop times whose stop_id isn't in stops.txt.
		stopId, ok := stopIdMap[stopTime.GtfsStopId]
		if !ok {
			continue
		}

		stops = append(stops, stopId)
		stopEvents = append(stopEvents, raptor.StopEvent{
			ArrivalTime:   stopTime.ArrivalTime,
			DepartureTime: stopTime.DepartureTime,
		})
	}

	if !slices.Equal(stops, rt.StopsForRoute(routeId)) {
		problems = append(problems, "stop_times stops != route stops")
	}

	if !slices.Equal(stopEvents, rt.StopEventsForTrip(routeId, tripIdx)) {
		problems = append(problems, "stop_times times != trip stop events")
	}

	return problems
}
