package raptor

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"router/pkg/gtfs"
	"router/pkg/transfer"
	"router/pkg/types"
	"router/pkg/utils"
)

func BuildRaptorTable(gtfsTable *gtfs.GTFSTable, date gtfs.GTFSDate) (*RaptorTable, error) {
	println("Building RAPTOR table")

	start := time.Now()

	gtfsStopIdMap := enumerateGtfsStops(gtfsTable.Stops)
	numStops := len(gtfsTable.Stops)
	selfTransfers := extractSelfTransfers(gtfsTable.Transfers, gtfsStopIdMap)
	transfers := transfer.CalculateTransfers(gtfsTable.Transfers, gtfsStopIdMap)

	gtfsActiveTripIdMap := enumerateGtfsTrips(gtfsTable.TripsForDate(date))

	raptorTrips := groupRaptorTrips(gtfsTable.StopTimes, gtfsStopIdMap, gtfsActiveTripIdMap)
	raptorRoutes := groupRaptorRoutes(raptorTrips)

	routes, stopIdsByRoute, tripsByRoute, stopEventsByRoute, firstStopIdOfRoute,
		firstTripOfRoute, numTripsInRoute, firstStopEventOfRoute, numRoutesForStop :=
		iterateOverRaptorRoutes(raptorRoutes, gtfsTable.RoutesById, gtfsTable.Trips, numStops)

	routeSegmentsByStop, firstRouteSegmentOfStop := groupRouteSegments(raptorRoutes, numRoutesForStop, numStops)

	table := RaptorTable{
		Stops:                   gtfsTable.Stops,
		Routes:                  routes,
		MinTransferTime:         selfTransfers,
		Transfers:               *transfers,
		StopIdsByRoute:          stopIdsByRoute,
		FirstStopIdOfRoute:      firstStopIdOfRoute,
		StopEventsByRoute:       stopEventsByRoute,
		FirstStopEventOfRoute:   firstStopEventOfRoute,
		NumTripsInRoute:         numTripsInRoute,
		RouteSegmentsByStop:     routeSegmentsByStop,
		FirstRouteSegmentOfStop: firstRouteSegmentOfStop,
		TripsByRoute:            tripsByRoute,
		FirstTripOfRoute:        firstTripOfRoute,
	}

	fmt.Printf("RAPTOR table built in %s\n", time.Since(start))

	return &table, nil
}

func enumerateGtfsStops(gtfsStops []gtfs.GTFSStop) map[gtfs.GTFSStopID]types.StopID {
	gtfsStopIdMap := make(map[gtfs.GTFSStopID]types.StopID, len(gtfsStops))
	for idx, stop := range gtfsStops {
		gtfsStopIdMap[stop.GtfsId] = types.StopID(idx)
	}

	return gtfsStopIdMap
}

func enumerateGtfsTrips(gtfsTrips []gtfs.GTFSTrip) map[gtfs.GTFSTripID]RaptorTripID {
	gtfsActiveTripIdMap := make(map[gtfs.GTFSTripID]RaptorTripID, len(gtfsTrips))

	for idx, trip := range gtfsTrips {
		gtfsActiveTripIdMap[trip.GtfsId] = RaptorTripID(idx)
	}

	return gtfsActiveTripIdMap
}

func extractSelfTransfers(
	gtfsTranfers []gtfs.GTFSTransfer,
	stopIdMap map[gtfs.GTFSStopID]types.StopID,
) StopTransferTimes {
	minTransferTime := make(StopTransferTimes, len(stopIdMap))

	for _, transfer := range gtfsTranfers {
		if transfer.TransferType != 2 {
			continue
		}

		if transfer.FromStopId != transfer.ToStopId {
			continue
		}

		stopId := stopIdMap[transfer.FromStopId]
		minTransferTime[stopId] = transfer.MinTransferTime
	}

	return minTransferTime
}

func groupRaptorTrips(
	gtfsStopTimes []gtfs.GTFSStopTime,
	stopIdMap map[gtfs.GTFSStopID]types.StopID,
	tripIdMap map[gtfs.GTFSTripID]RaptorTripID,
) []RaptorTrip {
	raptorTrips := make([]RaptorTrip, len(tripIdMap))

	for _, stopTime := range gtfsStopTimes {
		if tripId, ok := tripIdMap[stopTime.GtfsTripId]; ok {
			stopId, ok := stopIdMap[stopTime.GtfsStopId]
			if !ok {
				fmt.Printf("WARN: unknown stop_id %q\n", stopTime.GtfsStopId)
			}

			raptorTrips[tripId] = append(raptorTrips[tripId], RaptorStopTime{
				tripId:        tripId,
				stopId:        stopId,
				arrivalTime:   stopTime.ArrivalTime,
				departureTime: stopTime.DepartureTime,
				stopSequence:  stopTime.StopSequence,
			})
		}
	}

	for _, trip := range raptorTrips {
		slices.SortFunc(trip, func(stopTimeA, stopTimeB RaptorStopTime) int {
			return cmp.Compare(stopTimeA.stopSequence, stopTimeB.stopSequence)
		})
	}

	return raptorTrips
}

func getStopSequence(trip RaptorTrip) []types.StopID {
	return utils.Map(trip, func(stopTime RaptorStopTime) types.StopID {
		return stopTime.stopId
	})
}

func buildStopSequenceKey(stopSequence []types.StopID) string {
	return strings.Join(
		utils.Map(stopSequence, func(stopId types.StopID) string { return strconv.FormatUint(uint64(stopId), 10) }),
		",",
	)
}

func groupTripsByStopSequence(raptorTrips []RaptorTrip) map[string]*RaptorRoute {
	routeMap := make(map[string]*RaptorRoute)

	for _, trip := range raptorTrips {
		if len(trip) == 0 {
			continue
		}

		stopSequence := getStopSequence(trip)
		tripKey := buildStopSequenceKey(stopSequence)

		route, ok := routeMap[tripKey]
		if !ok {
			routeMap[tripKey] = &RaptorRoute{stopSequence, []RaptorTrip{trip}}
		} else {
			route.trips = append(route.trips, trip)
		}
	}

	fmt.Printf("Extracted %d raptor routes\n", len(routeMap))

	return routeMap
}

func sortRoutes(routeMap map[string]*RaptorRoute) []RaptorRoute {
	sortedSequenceKeys := slices.Sorted(maps.Keys(routeMap))

	routes := make([]RaptorRoute, len(routeMap))
	for i, key := range sortedSequenceKeys {
		routes[i] = *routeMap[key]
	}

	for _, route := range routes {
		slices.SortFunc(route.trips, func(tripA, tripB RaptorTrip) int {
			return cmp.Compare(tripA[0].departureTime, tripB[0].departureTime)
		})
	}

	return routes
}

func groupRaptorRoutes(raptorTrips []RaptorTrip) []RaptorRoute {
	return sortRoutes(groupTripsByStopSequence(raptorTrips))
}

func iterateOverRaptorRoutes(
	raptorRoutes []RaptorRoute,
	routeMap map[gtfs.GTFSRouteID]*gtfs.GTFSRoute,
	gtfsTrips []gtfs.GTFSTrip,
	numStops int,
) ([]gtfs.GTFSRoute, []types.StopID, []gtfs.GTFSTrip, []StopEvent, RouteStopOffsets,
	RouteTripOffsets, []uint32, RouteStopEventOffsets, []uint32) {
	numRoutes := len(raptorRoutes)

	routes := make([]gtfs.GTFSRoute, numRoutes)
	firstStopIdOfRoute := make(RouteStopOffsets, numRoutes+1)
	firstTripOfRoute := make(RouteTripOffsets, numRoutes+1)
	numTripsInRoute := make([]uint32, numRoutes)
	firstStopEventOfRoute := make(RouteStopEventOffsets, numRoutes+1)
	numRoutesForStop := make([]uint32, numStops)

	var stopIdsByRoute []types.StopID

	var stopEventsByRoute []StopEvent

	var tripsByRoute []gtfs.GTFSTrip

	for routeId, route := range raptorRoutes {
		firstTripId := route.trips[0][0].tripId
		firstTrip := gtfsTrips[firstTripId]
		gtfsRoute := *routeMap[firstTrip.GtfsRouteId]
		routes[routeId] = gtfsRoute

		firstStopIdOfRoute[routeId] = uint32(len(stopIdsByRoute))
		stopIdsByRoute = append(stopIdsByRoute, route.stopSequence...)

		firstTripOfRoute[routeId] = uint32(len(tripsByRoute))
		numTripsInRoute[routeId] = uint32(len(route.trips))

		firstStopEventOfRoute[routeId] = uint32(len(stopEventsByRoute))

		for _, trip := range route.trips {
			gtfsTrip := gtfsTrips[trip[0].tripId]
			tripsByRoute = append(tripsByRoute, gtfsTrip)

			for _, stopTime := range trip {
				stopEventsByRoute = append(stopEventsByRoute, StopEvent{
					ArrivalTime:   stopTime.arrivalTime,
					DepartureTime: stopTime.departureTime,
				})
			}
		}

		for _, stopId := range route.stopSequence {
			numRoutesForStop[stopId]++
		}
	}

	firstStopIdOfRoute[numRoutes] = uint32(len(stopIdsByRoute))
	firstTripOfRoute[numRoutes] = uint32(len(tripsByRoute))
	firstStopEventOfRoute[numRoutes] = uint32(len(stopEventsByRoute))

	return routes, stopIdsByRoute, tripsByRoute, stopEventsByRoute, firstStopIdOfRoute,
		firstTripOfRoute, numTripsInRoute, firstStopEventOfRoute, numRoutesForStop
}

func groupRouteSegments(
	raptorRoutes []RaptorRoute,
	numRoutesForStop []uint32,
	numStops int,
) ([]RouteSegment, StopRouteSegmentOffsets) {
	firstRouteSegmentOfStop := make(StopRouteSegmentOffsets, numStops+1)

	for s := range numStops {
		firstRouteSegmentOfStop[s+1] = firstRouteSegmentOfStop[s] + numRoutesForStop[s]
	}

	totalSegments := firstRouteSegmentOfStop[numStops]
	routeSegmentsByStop := make([]RouteSegment, totalSegments)

	cursor := make(StopRouteSegmentOffsets, numStops)
	copy(cursor, firstRouteSegmentOfStop[:numStops])

	for routeId, route := range raptorRoutes {
		for stopIdx, stopId := range route.stopSequence {
			routeSegmentsByStop[cursor[stopId]] = RouteSegment{
				RouteId:   types.RouteID(routeId),
				StopIndex: StopIndex(stopIdx),
			}
			cursor[stopId]++
		}
	}

	return routeSegmentsByStop, firstRouteSegmentOfStop
}
