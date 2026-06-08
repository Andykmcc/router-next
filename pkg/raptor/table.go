package raptor

import (
	"fmt"
	"router/pkg/gtfs"
	"router/pkg/transfer"
	"router/pkg/types"
	"router/pkg/utils"
)

type RaptorTripID uint32 // Temporary ID during processing. In the final RAPTOR table, trips are only stored per-route.

type RaptorStopTime struct {
	tripId        RaptorTripID
	stopId        types.StopID
	arrivalTime   types.Timestamp
	departureTime types.Timestamp
	stopSequence  uint32
}

type RaptorTrip []RaptorStopTime

type RaptorRoute struct {
	stopSequence []types.StopID
	trips        []RaptorTrip
}

type StopEvent struct {
	ArrivalTime   types.Timestamp
	DepartureTime types.Timestamp
}

type StopIndex uint32

type RouteSegment struct {
	RouteId   types.RouteID
	StopIndex StopIndex
}

type RouteStopOffsets []uint32        // indexed by types.RouteID
type RouteStopEventOffsets []uint32   // indexed by types.RouteID
type RouteTripOffsets []uint32        // indexed by types.RouteID
type StopRouteSegmentOffsets []uint32 // indexed by types.StopID
type StopTransferTimes []uint32       // indexed by types.StopID

type RaptorTable struct {
	Stops  []gtfs.GTFSStop
	Routes []gtfs.GTFSRoute

	MinTransferTime StopTransferTimes
	Transfers       transfer.TransferTable

	StopIdsByRoute     []types.StopID
	FirstStopIdOfRoute RouteStopOffsets

	TripsByRoute     []gtfs.GTFSTrip
	FirstTripOfRoute RouteTripOffsets
	NumTripsInRoute  []uint32 // indexed by types.RouteID

	StopEventsByRoute     []StopEvent
	FirstStopEventOfRoute RouteStopEventOffsets

	RouteSegmentsByStop     []RouteSegment
	FirstRouteSegmentOfStop StopRouteSegmentOffsets
}

func (rt *RaptorTable) SnapshotString() string {
	return fmt.Sprintf(
		`
MinTransferTime: %d
---------------------------------------------------------------
%s

StopIdsByRoute: %d
---------------------------------------------------------------
%s

FirstStopIdOfRoute: %d
---------------------------------------------------------------
%s

FirstTripOfRoute: %d
---------------------------------------------------------------
%s

NumTripsInRoute: %d
---------------------------------------------------------------
%s

FirstStopEventOfRoute: %d
---------------------------------------------------------------
%s

FirstRouteSegmentOfStop: %d
---------------------------------------------------------------
%s
`,
		len(rt.MinTransferTime),
		utils.SnapshotStr(rt.MinTransferTime),
		len(rt.StopIdsByRoute),
		utils.SnapshotStr(rt.StopIdsByRoute),
		len(rt.FirstStopIdOfRoute),
		utils.SnapshotStr(rt.FirstStopIdOfRoute),
		len(rt.FirstTripOfRoute),
		utils.SnapshotStr(rt.FirstTripOfRoute),
		len(rt.NumTripsInRoute),
		utils.SnapshotStr(rt.NumTripsInRoute),
		len(rt.FirstStopEventOfRoute),
		utils.SnapshotStr(rt.FirstStopEventOfRoute),
		len(rt.FirstRouteSegmentOfStop),
		utils.SnapshotStr(rt.FirstRouteSegmentOfStop),
	)
}

func (rt *RaptorTable) NumStops() int  { return len(rt.Stops) }
func (rt *RaptorTable) NumRoutes() int { return len(rt.NumTripsInRoute) }

func (rt *RaptorTable) StopsForRoute(route types.RouteID) []types.StopID {
	start := rt.FirstStopIdOfRoute[route]
	end := rt.FirstStopIdOfRoute[route+1]

	return rt.StopIdsByRoute[start:end]
}

func (rt *RaptorTable) NumStopsInRoute(route types.RouteID) uint32 {
	return uint32(len(rt.StopsForRoute(route)))
}

func (rt *RaptorTable) StopEventsForTrip(route types.RouteID, trip uint32) []StopEvent {
	numStops := rt.NumStopsInRoute(route)
	routeStart := rt.FirstStopEventOfRoute[route]
	tripStart := routeStart + numStops*trip

	return rt.StopEventsByRoute[tripStart : tripStart+numStops]
}

func (rt *RaptorTable) TripInRoute(route types.RouteID, trip uint32) gtfs.GTFSTrip {
	routeStart := rt.FirstTripOfRoute[route]

	return rt.TripsByRoute[routeStart+trip]
}

func (rt *RaptorTable) RoutesForStop(stop types.StopID) []RouteSegment {
	start := rt.FirstRouteSegmentOfStop[stop]
	end := rt.FirstRouteSegmentOfStop[stop+1]

	return rt.RouteSegmentsByStop[start:end]
}

func (rt *RaptorTable) Sizeof() int {
	sizeTable := utils.SizeOf[gtfs.GTFSTable]()

	sizeStops := rt.NumStops() * utils.SizeOf[gtfs.GTFSStop]()
	sizeRoutes := rt.NumRoutes() * utils.SizeOf[gtfs.GTFSRoute]()
	sizeTranfers := len(rt.MinTransferTime) * 4
	sizeStopIds := (len(rt.StopIdsByRoute) + rt.NumStops()) * 4
	sizeTrips := len(rt.TripsByRoute)*utils.SizeOf[gtfs.GTFSTrip]() + 2*rt.NumRoutes()*4
	sizeStopEvents := len(rt.StopEventsByRoute)*utils.SizeOf[StopEvent]() + rt.NumRoutes()*4
	sizeRouteSegments := len(rt.RouteSegmentsByStop)*utils.SizeOf[RouteSegment]() + rt.NumStops()*4

	totalBytes :=
		sizeTable + sizeStops + sizeRoutes + sizeTranfers +
			sizeStopIds + sizeTrips + sizeStopEvents + sizeRouteSegments

	return totalBytes
}
