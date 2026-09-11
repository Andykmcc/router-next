package raptor

import (
	"encoding/binary"
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

// u32ColBytes serialises a uint32-backed column to little-endian bytes for a
// deterministic checksum.
func u32ColBytes[T ~uint32](col []T) []byte {
	buf := make([]byte, 0, len(col)*4)
	for _, v := range col {
		buf = binary.LittleEndian.AppendUint32(buf, uint32(v))
	}

	return buf
}

// modeColBytes serialises the 1-byte-per-element transfer-mode column.
func modeColBytes(col []types.TransferMode) []byte {
	buf := make([]byte, len(col))
	for i, v := range col {
		buf[i] = byte(v)
	}

	return buf
}

// dualWeightColBytes serialises DualWeight pairs to little-endian bytes.
func dualWeightColBytes(col []types.DualWeight) []byte {
	buf := make([]byte, 0, len(col)*8)
	for _, w := range col {
		buf = binary.LittleEndian.AppendUint32(buf, w.RealTime)
		buf = binary.LittleEndian.AppendUint32(buf, w.PenalizedCost)
	}

	return buf
}

// sampleEnds returns the first and last n elements of col for a head/tail
// snapshot sample. For a column of n or fewer elements it returns the whole
// column as head and nil as tail.
func sampleEnds[T any](col []T, n int) (head, tail []T) {
	if len(col) <= n {
		return col, nil
	}

	return col[:n], col[len(col)-n:]
}

func (rt *RaptorTable) SnapshotString() string {
	tgtHead, tgtTail := sampleEnds(rt.Transfers.TransferTarget, utils.SnapshotLineWidth)
	modeHead, modeTail := sampleEnds(rt.Transfers.TransferModes, utils.SnapshotLineWidth)
	wHead, wTail := sampleEnds(rt.Transfers.TransferWeights, utils.SnapshotLineWidth)

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

OffsetOfStop: %d
---------------------------------------------------------------
%s

TransferTarget: %d checksum=%s
---------------------------------------------------------------
head: %s
tail: %s

TransferModes: %d checksum=%s
---------------------------------------------------------------
head: %s
tail: %s

TransferWeights: %d checksum=%s
---------------------------------------------------------------
head: %s
tail: %s
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
		len(rt.Transfers.OffsetOfStop),
		utils.SnapshotStr(rt.Transfers.OffsetOfStop),
		len(rt.Transfers.TransferTarget),
		utils.FNV64(u32ColBytes(rt.Transfers.TransferTarget)),
		utils.SnapshotStr(tgtHead),
		utils.SnapshotStr(tgtTail),
		len(rt.Transfers.TransferModes),
		utils.FNV64(modeColBytes(rt.Transfers.TransferModes)),
		utils.SnapshotStr(modeHead),
		utils.SnapshotStr(modeTail),
		len(rt.Transfers.TransferWeights),
		utils.FNV64(dualWeightColBytes(rt.Transfers.TransferWeights)),
		utils.SnapshotDualWeights(wHead),
		utils.SnapshotDualWeights(wTail),
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
	sizeTransferCSR := len(rt.Transfers.OffsetOfStop)*4 + len(rt.Transfers.TransferTarget)*4 +
		len(rt.Transfers.TransferModes)*1 + len(rt.Transfers.TransferWeights)*8

	totalBytes :=
		sizeTable + sizeStops + sizeRoutes + sizeTranfers +
			sizeStopIds + sizeTrips + sizeStopEvents + sizeRouteSegments + sizeTransferCSR

	return totalBytes
}
