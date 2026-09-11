package debug

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"router/pkg/gtfs"
	"router/pkg/raptor"
	"router/pkg/transfer"
	"router/pkg/types"
)

func miniTable() *raptor.RaptorTable {
	return &raptor.RaptorTable{
		Stops: []gtfs.GTFSStop{
			{GtfsId: "a", Name: "Alpha", Lat: 37.10, Lon: -122.10},
			{GtfsId: "b", Name: "Bravo", Lat: 37.20, Lon: -122.20},
			{GtfsId: "c", Name: "Charlie", Lat: 37.30, Lon: -122.30},
		},
		Routes: []gtfs.GTFSRoute{
			{GtfsId: "R", GtfsAgencyId: "AG", ShortName: "R", LongName: "Rapid", RouteType: 3, Color: ""},
			// GtfsId contains ':' and '/' like real merged-feed route ids
			// (e.g. "EM:Shell/Pow", "SF:15") - exercises routeIndexByGtfsId
			// and the query-param (not path-segment) route lookup.
			{GtfsId: "EM:Shell/Pow", GtfsAgencyId: "AG", ShortName: "15", LongName: "Shell to Powell", RouteType: 3, Color: ""},
		},
		MinTransferTime: raptor.StopTransferTimes{0, 0, 0},
		Transfers: transfer.TransferTable{
			OffsetOfStop:    make([]uint32, 4), // len(Stops)+1 == 3+1
			TransferTarget:  nil,
			TransferModes:   nil,
			TransferWeights: nil,
		},
		StopIdsByRoute:          []types.StopID{0, 1, 2},
		FirstStopIdOfRoute:      raptor.RouteStopOffsets{0, 2, 3},
		TripsByRoute:            nil,
		FirstTripOfRoute:        raptor.RouteTripOffsets{0, 0},
		NumTripsInRoute:         []uint32{1},
		StopEventsByRoute:       nil,
		FirstStopEventOfRoute:   raptor.RouteStopEventOffsets{0, 0},
		RouteSegmentsByStop:     nil,
		FirstRouteSegmentOfStop: raptor.StopRouteSegmentOffsets{0, 0, 0, 0},
	}
}

func TestParseClock(t *testing.T) {
	cases := map[string]types.Timestamp{
		"08:00":    28800,
		"08:00:30": 28830,
		"00:00:00": 0,
		"23:59":    86340,
	}
	for in, want := range cases {
		got, err := parseClock(in)
		if err != nil || got != want {
			t.Errorf("parseClock(%q) = %d, %v; want %d", in, got, err, want)
		}
	}

	if _, err := parseClock("nope"); err == nil {
		t.Error("parseClock(\"nope\") should error")
	}
}

func TestBuildRouteResponse(t *testing.T) {
	rt := miniTable()

	journeys := []raptor.Journey{{
		NumLegs:     2,
		ArrivalTime: 29220,
		Legs: []raptor.Label{
			// transit A(0) -> B(1): alight index 1 of route 0's stop list
			{RouteId: 0, TripIdx: 0, BoardStopId: 0, BoardStopIdx: 0, AlightStopIdx: 1,
				TransferMode: types.TransferModeNone, TransferToStopId: 0,
				TransferWeight: types.DualWeight{RealTime: 0, PenalizedCost: 0}},
			// walk B(1) -> C(2)
			{RouteId: 0, TripIdx: 0, BoardStopId: 1, BoardStopIdx: 0, AlightStopIdx: 0,
				TransferMode: types.TransferModeWalk, TransferToStopId: 2,
				TransferWeight: types.DualWeight{RealTime: 120, PenalizedCost: 120}},
		},
	}}

	resp := buildRouteResponse(rt, journeys)

	if len(resp.Journeys) != 1 {
		t.Fatalf("want 1 journey, got %d", len(resp.Journeys))
	}

	j := resp.Journeys[0]
	if j.ArrivalTime != 29220 || j.NumTransitLegs != 1 || len(j.Legs) != 2 {
		t.Fatalf("journey summary wrong: %+v", j)
	}

	if j.Legs[0].Kind != "transit" || j.Legs[0].Route == nil || j.Legs[0].Route.ShortName != "R" ||
		j.Legs[0].From.Name != "Alpha" || j.Legs[0].To.Name != "Bravo" {
		t.Fatalf("transit leg view wrong: %+v", j.Legs[0])
	}

	if j.Legs[1].Kind != "walk" || j.Legs[1].Route != nil ||
		j.Legs[1].From.Name != "Bravo" || j.Legs[1].To.Name != "Charlie" ||
		j.Legs[1].TransferSeconds != 120 {
		t.Fatalf("walk leg view wrong: %+v", j.Legs[1])
	}
}

func TestHandleRouteBadParams(t *testing.T) {
	h := handleRoute(miniTable())

	cases := map[string]int{
		// missing start/end is still a 400 (a missing time is not, on its own)
		"/route":                           http.StatusBadRequest,
		"/route?start=x&end=1&time=08:00":  http.StatusBadRequest,
		"/route?start=0&end=99&time=08:00": http.StatusBadRequest,
		"/route?start=0&end=1&time=nope":   http.StatusBadRequest,
		// an absent time defaults to the host clock, so this is a 200
		"/route?start=0&end=1": http.StatusOK,
	}

	for target, want := range cases {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if rec.Code != want {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, want)
		}
	}
}

func TestHandleRouteOK(t *testing.T) {
	h := handleRoute(miniTable())
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/route?start=0&end=1&time=08:00", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body routeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
}
