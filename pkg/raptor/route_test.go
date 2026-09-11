package raptor

import (
	"slices"
	"testing"

	"router/pkg/gtfs"
	"router/pkg/transfer"
	"router/pkg/types"
)

// parentsGrid builds a [MAX_ROUNDS+1][numStops]Label grid of zero Labels.
func parentsGrid(numStops int) [][]Label {
	p := make([][]Label, MAX_ROUNDS+1)
	for k := range p {
		p[k] = make([]Label, numStops)
	}

	return p
}

func TestReconstructLegsTransitThenTransfer(t *testing.T) {
	// round 1: transit 0->1 (parents[1][1]); transfer 1->2 same round (parents[1][2]).
	p := parentsGrid(3)
	p[1][1] = transitLabel(0, 0, 0, 0, 1)
	p[1][2] = transferLabel(1, 2, types.DualWeight{RealTime: 60, PenalizedCost: 60}, types.TransferModeWalk)

	legs := reconstructLegs(0, 2, 1, 3, p)

	if len(legs) != 2 {
		t.Fatalf("len(legs) = %d, want 2 (%+v)", len(legs), legs)
	}

	if legs[0].IsTransfer() || legs[0].BoardStopId != 0 || legs[0].AlightStopIdx != 1 {
		t.Fatalf("legs[0] should be transit 0->1, got %+v", legs[0])
	}

	if !legs[1].IsTransfer() || legs[1].BoardStopId != 1 || legs[1].TransferToStopId != 2 ||
		legs[1].TransferMode != types.TransferModeWalk || legs[1].TransferWeight.RealTime != 60 {
		t.Fatalf("legs[1] should be walk 1->2, got %+v", legs[1])
	}
}

func TestReconstructLegsOriginTransfer(t *testing.T) {
	// pre-loop origin walk 0->1 lives in parents[0]; transit 1->2 in parents[1].
	p := parentsGrid(3)
	p[0][1] = transferLabel(0, 1, types.DualWeight{RealTime: 90, PenalizedCost: 90}, types.TransferModeWalk)
	p[1][2] = transitLabel(0, 0, 1, 0, 1)

	legs := reconstructLegs(0, 2, 1, 3, p)

	if len(legs) != 2 {
		t.Fatalf("len(legs) = %d, want 2 (%+v)", len(legs), legs)
	}

	if !legs[0].IsTransfer() || legs[0].BoardStopId != 0 || legs[0].TransferToStopId != 1 {
		t.Fatalf("legs[0] should be walk 0->1, got %+v", legs[0])
	}

	if legs[1].IsTransfer() || legs[1].BoardStopId != 1 {
		t.Fatalf("legs[1] should be transit 1->2, got %+v", legs[1])
	}
}

func TestReconstructLegsChainedTransfers(t *testing.T) {
	// origin walk 0->1->2 (both in parents[0]), then transit 2->3 in parents[1].
	p := parentsGrid(4)
	p[0][1] = transferLabel(0, 1, types.DualWeight{RealTime: 30, PenalizedCost: 30}, types.TransferModeWalk)
	p[0][2] = transferLabel(1, 2, types.DualWeight{RealTime: 30, PenalizedCost: 30}, types.TransferModeWalk)
	p[1][3] = transitLabel(0, 0, 2, 0, 1)

	legs := reconstructLegs(0, 3, 1, 4, p)

	if len(legs) != 3 {
		t.Fatalf("len(legs) = %d, want 3 (%+v)", len(legs), legs)
	}

	if legs[0].BoardStopId != 0 || legs[0].TransferToStopId != 1 ||
		legs[1].BoardStopId != 1 || legs[1].TransferToStopId != 2 ||
		legs[2].IsTransfer() || legs[2].BoardStopId != 2 {
		t.Fatalf("unexpected leg chain: %+v", legs)
	}
}

type testTrip = []StopEvent

type testRoute struct {
	stops []types.StopID
	trips []testTrip
}

type testTransfer struct {
	from    types.StopID
	to      types.StopID
	seconds uint32
	mode    types.TransferMode
}

// evTrip builds a trip whose arrival == departure == the given absolute
// seconds-since-midnight at each successive stop of its route.
func evTrip(times ...types.Timestamp) testTrip {
	events := make([]StopEvent, len(times))
	for i, tm := range times {
		events[i] = StopEvent{ArrivalTime: tm, DepartureTime: tm}
	}

	return events
}

// newTestTable hand-builds a minimal RaptorTable exercised by Route: routes are
// laid out into StopIdsByRoute / StopEventsByRoute CSR arrays, route segments are
// grouped by stop, and transfers are flattened into the transfer CSR sorted by
// (target, mode). Stops carry no coordinates; MinTransferTime is all zero.
func newTestTable(numStops int, routes []testRoute, transfers []testTransfer) *RaptorTable {
	rt := &RaptorTable{
		Stops:           make([]gtfs.GTFSStop, numStops),
		Routes:          make([]gtfs.GTFSRoute, len(routes)),
		MinTransferTime: make(StopTransferTimes, numStops),
		Transfers: transfer.TransferTable{
			OffsetOfStop:    nil,
			TransferTarget:  nil,
			TransferModes:   nil,
			TransferWeights: nil,
		},
		StopIdsByRoute:          nil,
		FirstStopIdOfRoute:      make(RouteStopOffsets, len(routes)+1),
		TripsByRoute:            nil,
		FirstTripOfRoute:        make(RouteTripOffsets, len(routes)+1),
		NumTripsInRoute:         make([]uint32, len(routes)),
		StopEventsByRoute:       nil,
		FirstStopEventOfRoute:   make(RouteStopEventOffsets, len(routes)+1),
		RouteSegmentsByStop:     nil,
		FirstRouteSegmentOfStop: make(StopRouteSegmentOffsets, numStops+1),
	}

	numRoutesForStop := make([]uint32, numStops)

	for r, route := range routes {
		rt.FirstStopIdOfRoute[r] = uint32(len(rt.StopIdsByRoute))
		rt.StopIdsByRoute = append(rt.StopIdsByRoute, route.stops...)

		rt.FirstStopEventOfRoute[r] = uint32(len(rt.StopEventsByRoute))
		rt.NumTripsInRoute[r] = uint32(len(route.trips))

		for _, trip := range route.trips {
			rt.StopEventsByRoute = append(rt.StopEventsByRoute, trip...)
		}

		for _, s := range route.stops {
			numRoutesForStop[s]++
		}
	}

	rt.FirstStopIdOfRoute[len(routes)] = uint32(len(rt.StopIdsByRoute))
	rt.FirstStopEventOfRoute[len(routes)] = uint32(len(rt.StopEventsByRoute))

	for s := range numStops {
		rt.FirstRouteSegmentOfStop[s+1] = rt.FirstRouteSegmentOfStop[s] + numRoutesForStop[s]
	}

	rt.RouteSegmentsByStop = make([]RouteSegment, rt.FirstRouteSegmentOfStop[numStops])
	cursor := make([]uint32, numStops)
	copy(cursor, rt.FirstRouteSegmentOfStop[:numStops])

	for r, route := range routes {
		for stopIdx, s := range route.stops {
			rt.RouteSegmentsByStop[cursor[s]] = RouteSegment{
				RouteId:   types.RouteID(r),
				StopIndex: StopIndex(stopIdx),
			}
			cursor[s]++
		}
	}

	rt.Transfers = buildTestTransfers(numStops, transfers)

	return rt
}

func buildTestTransfers(numStops int, transfers []testTransfer) transfer.TransferTable {
	adjacency := make([][]testTransfer, numStops)
	for _, tr := range transfers {
		adjacency[tr.from] = append(adjacency[tr.from], tr)
	}

	offsets := make([]uint32, numStops+1)

	var (
		targets []types.StopID
		modes   []types.TransferMode
		weights []types.DualWeight
	)

	for s := range numStops {
		edges := adjacency[s]
		slices.SortFunc(edges, func(a, b testTransfer) int {
			if a.to != b.to {
				return int(a.to) - int(b.to)
			}

			return int(a.mode) - int(b.mode)
		})

		for _, tr := range edges {
			targets = append(targets, tr.to)
			modes = append(modes, tr.mode)
			weights = append(weights, types.DualWeight{RealTime: tr.seconds, PenalizedCost: tr.seconds})
		}

		offsets[s+1] = uint32(len(targets))
	}

	return transfer.TransferTable{
		OffsetOfStop:    offsets,
		TransferTarget:  targets,
		TransferModes:   modes,
		TransferWeights: weights,
	}
}

// maxConsecutiveTransferSeconds returns the largest summed TransferWeight.RealTime
// over any run of back-to-back connecting legs in the journey. A transit leg
// resets the run.
func maxConsecutiveTransferSeconds(j Journey) uint32 {
	var maxRun, run uint32

	for _, leg := range j.Legs {
		if leg.IsTransfer() {
			run += leg.TransferWeight.RealTime
			if run > maxRun {
				maxRun = run
			}

			continue
		}

		run = 0
	}

	return maxRun
}

// countTransitLegs returns how many of a journey's legs are transit (not transfers).
func countTransitLegs(j Journey) int {
	n := 0

	for _, leg := range j.Legs {
		if !leg.IsTransfer() {
			n++
		}
	}

	return n
}

func TestRouteTransferBudget(t *testing.T) { // FIX 1 regression
	// Walkable chain 0..9 with 250 s hops each direction: 4 chained hops (1000 s)
	// exceed MaxTransferSeconds (900), 3 hops (750 s) do not. One bus 0->4 gives
	// a journey to stop 6 via bus + a bounded 2-hop walk (500 s). Stop 9 is on no
	// route and sits 9 hops (2250 s) from the origin on foot.
	const hop = 250

	var transfers []testTransfer

	for s := types.StopID(0); s < 9; s++ {
		transfers = append(transfers,
			testTransfer{from: s, to: s + 1, seconds: hop, mode: types.TransferModeWalk},
			testTransfer{from: s + 1, to: s, seconds: hop, mode: types.TransferModeWalk},
		)
	}

	rt := newTestTable(10, []testRoute{
		{stops: []types.StopID{0, 4}, trips: []testTrip{evTrip(28800, 29000)}},
	}, transfers)

	journeys := rt.Route(0, 6, 28800)
	if len(journeys) == 0 {
		t.Fatal("want at least one journey to stop 6")
	}

	sawTransfer := false

	for i, j := range journeys {
		if got := maxConsecutiveTransferSeconds(j); got > MaxTransferSeconds {
			t.Errorf("journey %d: consecutive transfer run = %d s exceeds MaxTransferSeconds (%d): %+v",
				i, got, MaxTransferSeconds, j.Legs)
		}

		for _, leg := range j.Legs {
			if leg.IsTransfer() {
				sawTransfer = true
			}
		}
	}

	if !sawTransfer {
		t.Fatalf("expected the bus+walk journey to stop 6 to contain a connecting leg: %+v", journeys)
	}

	// Stop 9 is reachable from the origin only by a 9-hop (2250 s) walk chain and
	// is on no route: the budget must keep it out of the result entirely.
	if far := rt.Route(0, 9, 28800); len(far) != 0 {
		t.Fatalf("want no journey to stop 9 (walk chain %d s > budget %d), got %d: %+v",
			9*hop, MaxTransferSeconds, len(far), far)
	}
}

func TestRouteBoardsEarliestTrip(t *testing.T) { // D regression
	// Two trips on one route (already in departure order, as sortRoutes leaves
	// them). A 600 s origin walk reaches stop 1 at 29400 — after the early
	// trip's 29100 departure there but before the late trip's 30100 — so the
	// downstream boarding opportunity must NOT switch us off the
	// already-boarded earlier trip (which arrives at stop 2 at 29200).
	late := evTrip(30000, 30100, 30200)
	early := evTrip(29000, 29100, 29200)

	rt := newTestTable(3, []testRoute{
		{stops: []types.StopID{0, 1, 2}, trips: []testTrip{early, late}},
	}, []testTransfer{
		{from: 0, to: 1, seconds: 600, mode: types.TransferModeWalk},
		{from: 1, to: 0, seconds: 600, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 2, 28800)

	if len(journeys) != 1 {
		t.Fatalf("len(journeys) = %d, want 1 (%+v)", len(journeys), journeys)
	}

	j := journeys[0]
	if j.ArrivalTime != 29200 {
		t.Fatalf("ArrivalTime = %d, want 29200 (boarded the late trip?)", j.ArrivalTime)
	}

	if len(j.Legs) != 1 || j.Legs[0].IsTransfer() || j.Legs[0].BoardStopId != 0 || j.Legs[0].AlightStopIdx != 2 {
		t.Fatalf("want one transit leg 0->2, got %+v", j.Legs)
	}
}

func TestRoutePureTransit(t *testing.T) {
	// One route through stops 0,1,2; one trip departing 0 at 08:00:00.
	rt := newTestTable(3, []testRoute{
		{stops: []types.StopID{0, 1, 2}, trips: []testTrip{evTrip(28800, 29100, 29400)}},
	}, nil)

	journeys := rt.Route(0, 2, 28800)

	if len(journeys) != 1 {
		t.Fatalf("len(journeys) = %d, want 1 (%+v)", len(journeys), journeys)
	}

	j := journeys[0]
	if j.ArrivalTime != 29400 {
		t.Fatalf("ArrivalTime = %d, want 29400", j.ArrivalTime)
	}

	if j.NumLegs != 1 || len(j.Legs) != 1 || j.Legs[0].IsTransfer() {
		t.Fatalf("want one transit leg, got %+v", j.Legs)
	}

	if j.Legs[0].BoardStopId != 0 || j.Legs[0].AlightStopIdx != 2 {
		t.Fatalf("leg board/alight wrong: %+v", j.Legs[0])
	}
}

func TestRouteTransitThenWalk(t *testing.T) { // F1
	rt := newTestTable(3, []testRoute{
		{stops: []types.StopID{0, 1}, trips: []testTrip{evTrip(28800, 29100)}},
	}, []testTransfer{
		{from: 1, to: 2, seconds: 120, mode: types.TransferModeWalk},
		{from: 2, to: 1, seconds: 120, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 2, 28800)

	if len(journeys) != 1 {
		t.Fatalf("len(journeys) = %d, want 1", len(journeys))
	}

	j := journeys[0]
	if j.ArrivalTime != 29220 { // 29100 + 120
		t.Fatalf("ArrivalTime = %d, want 29220", j.ArrivalTime)
	}

	if len(j.Legs) != 2 || j.NumLegs != 2 {
		t.Fatalf("want 2 legs, got %+v", j.Legs)
	}

	if j.Legs[0].IsTransfer() || j.Legs[0].BoardStopId != 0 || j.Legs[0].AlightStopIdx != 1 {
		t.Fatalf("legs[0] not transit 0->1: %+v", j.Legs[0])
	}

	if !j.Legs[1].IsTransfer() || j.Legs[1].TransferMode != types.TransferModeWalk ||
		j.Legs[1].BoardStopId != 1 || j.Legs[1].TransferToStopId != 2 ||
		j.Legs[1].TransferWeight.RealTime != 120 {
		t.Fatalf("legs[1] not walk 1->2: %+v", j.Legs[1])
	}
}

func TestRouteWalkFromOrigin(t *testing.T) { // F3
	rt := newTestTable(3, []testRoute{
		{stops: []types.StopID{1, 2}, trips: []testTrip{evTrip(29100, 29400)}},
	}, []testTransfer{
		{from: 0, to: 1, seconds: 120, mode: types.TransferModeWalk},
		{from: 1, to: 0, seconds: 120, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 2, 28800)

	if len(journeys) != 1 {
		t.Fatalf("len(journeys) = %d, want 1 (F3 origin walk missed?)", len(journeys))
	}

	j := journeys[0]
	if j.ArrivalTime != 29400 || len(j.Legs) != 2 {
		t.Fatalf("want walk+transit arriving 29400, got %+v", j)
	}

	if !j.Legs[0].IsTransfer() || j.Legs[0].BoardStopId != 0 || j.Legs[0].TransferToStopId != 1 {
		t.Fatalf("legs[0] not walk 0->1: %+v", j.Legs[0])
	}

	if j.Legs[1].IsTransfer() || j.Legs[1].BoardStopId != 1 {
		t.Fatalf("legs[1] not transit 1->2: %+v", j.Legs[1])
	}
}

func TestRouteChainedWalks(t *testing.T) { // F4
	rt := newTestTable(4, []testRoute{
		{stops: []types.StopID{2, 3}, trips: []testTrip{evTrip(29200, 29500)}},
	}, []testTransfer{
		{from: 0, to: 1, seconds: 120, mode: types.TransferModeWalk},
		{from: 1, to: 0, seconds: 120, mode: types.TransferModeWalk},
		{from: 1, to: 2, seconds: 120, mode: types.TransferModeWalk},
		{from: 2, to: 1, seconds: 120, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 3, 28800)

	if len(journeys) != 1 {
		t.Fatalf("len(journeys) = %d, want 1 (F4 chained walk missed?)", len(journeys))
	}

	j := journeys[0]
	if len(j.Legs) != 3 || j.ArrivalTime != 29500 {
		t.Fatalf("want walk,walk,transit arriving 29500, got %+v", j)
	}

	if j.Legs[0].BoardStopId != 0 || j.Legs[0].TransferToStopId != 1 ||
		j.Legs[1].BoardStopId != 1 || j.Legs[1].TransferToStopId != 2 ||
		j.Legs[2].IsTransfer() || j.Legs[2].BoardStopId != 2 {
		t.Fatalf("unexpected leg chain: %+v", j.Legs)
	}
}

func TestRouteParetoFrontier(t *testing.T) { // T5
	// route 0 (X): stop 0 -> stop 3 direct, slow (arrive 30000)
	// route 1 (Y): stop 0 -> stop 1 (arrive 29000)
	// walk 1 -> 2 (60 s); route 2 (Z): stop 2 -> stop 3 (arrive 29300)
	rt := newTestTable(4, []testRoute{
		{stops: []types.StopID{0, 3}, trips: []testTrip{evTrip(28800, 30000)}},
		{stops: []types.StopID{0, 1}, trips: []testTrip{evTrip(28800, 29000)}},
		{stops: []types.StopID{2, 3}, trips: []testTrip{evTrip(29200, 29300)}},
	}, []testTransfer{
		{from: 1, to: 2, seconds: 60, mode: types.TransferModeWalk},
		{from: 2, to: 1, seconds: 60, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 3, 28800)

	if len(journeys) != 2 {
		t.Fatalf("len(journeys) = %d, want 2 (%+v)", len(journeys), journeys)
	}

	if journeys[0].ArrivalTime != 30000 || countTransitLegs(journeys[0]) != 1 {
		t.Fatalf("journey[0] want direct/1-leg/30000, got %+v", journeys[0])
	}

	if journeys[1].ArrivalTime != 29300 || countTransitLegs(journeys[1]) != 2 || len(journeys[1].Legs) != 3 {
		t.Fatalf("journey[1] want 2-transit+walk/29300, got %+v", journeys[1])
	}
}

func TestRouteTrailingWalkReconstruction(t *testing.T) { // T6
	// Same as pareto, plus stop 4 reachable only by a walk from stop 3.
	rt := newTestTable(5, []testRoute{
		{stops: []types.StopID{0, 3}, trips: []testTrip{evTrip(28800, 30000)}},
		{stops: []types.StopID{0, 1}, trips: []testTrip{evTrip(28800, 29000)}},
		{stops: []types.StopID{2, 3}, trips: []testTrip{evTrip(29200, 29300)}},
	}, []testTransfer{
		{from: 1, to: 2, seconds: 60, mode: types.TransferModeWalk},
		{from: 2, to: 1, seconds: 60, mode: types.TransferModeWalk},
		{from: 3, to: 4, seconds: 90, mode: types.TransferModeWalk},
		{from: 4, to: 3, seconds: 90, mode: types.TransferModeWalk},
	})

	journeys := rt.Route(0, 4, 28800)

	if len(journeys) == 0 {
		t.Fatal("want at least one journey to stop 4")
	}

	best := journeys[len(journeys)-1] // fastest / most-legs frontier point
	if best.NumLegs != len(best.Legs) {
		t.Fatalf("NumLegs (%d) != len(Legs) (%d)", best.NumLegs, len(best.Legs))
	}

	if best.Legs[len(best.Legs)-1].TransferToStopId != 4 || !best.Legs[len(best.Legs)-1].IsTransfer() {
		t.Fatalf("last leg should be a walk to 4: %+v", best.Legs)
	}

	if countTransitLegs(best) != 2 {
		t.Fatalf("want 2 transit legs in %+v", best.Legs)
	}
}
