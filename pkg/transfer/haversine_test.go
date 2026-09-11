package transfer

import (
	"math"
	"testing"

	"router/pkg/gtfs"
	"router/pkg/types"
)

func approx(t *testing.T, got, want, tol float64, label string) {
	t.Helper()

	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %v, want %v (±%v)", label, got, want, tol)
	}
}

func TestHaversineMeters(t *testing.T) {
	// One degree of latitude is ~111.19 km anywhere on the sphere used here.
	approx(t, haversineMeters(0, 0, 1, 0), 111_194.9, 5, "1 deg latitude")

	// Same point => zero.
	approx(t, haversineMeters(37.75, -122.42, 37.75, -122.42), 0, 1e-6, "identical points")

	// Symmetry.
	a := haversineMeters(37.75, -122.42, 37.80, -122.40)
	b := haversineMeters(37.80, -122.40, 37.75, -122.42)
	approx(t, a, b, 1e-9, "symmetry")
}

func TestDegreesToRadians(t *testing.T) {
	approx(t, degreesToRadians(180), math.Pi, 1e-12, "180 deg")
	approx(t, degreesToRadians(0), 0, 1e-12, "0 deg")
}

func TestProjection(t *testing.T) {
	p := projection{lat0: 37.75, lon0: -122.42}

	x, y := p.project(37.75, -122.42)
	approx(t, x, 0, 1e-6, "origin x")
	approx(t, y, 0, 1e-6, "origin y")

	// 500 m due north: dLat = degrees(500 / earthRadius).
	dLat := (500.0 / earthRadius) * 180 / math.Pi
	x, y = p.project(37.75+dLat, -122.42)
	approx(t, x, 0, 1e-3, "north x")
	approx(t, y, 500, 1e-3, "north y")

	// 500 m due east: dLon = degrees(500 / (earthRadius * cos(lat0))).
	dLon := (500.0 / (earthRadius * math.Cos(degreesToRadians(37.75)))) * 180 / math.Pi
	x, y = p.project(37.75, -122.42+dLon)
	approx(t, x, 500, 1e-3, "east x")
	approx(t, y, 0, 1e-3, "east y")
}

func TestNewProjectionMean(t *testing.T) {
	stops := []gtfs.GTFSStop{
		{GtfsId: "a", Name: "A", Lat: 37.0, Lon: -122.0},
		{GtfsId: "b", Name: "B", Lat: 38.0, Lon: -123.0},
	}

	p := newProjection(stops)
	approx(t, p.lat0, 37.5, 1e-12, "mean lat")
	approx(t, p.lon0, -122.5, 1e-12, "mean lon")
}

func TestCalculateTransfersEmpty(t *testing.T) {
	tt := CalculateTransfers(nil)

	if len(tt.OffsetOfStop) != 1 || tt.OffsetOfStop[0] != 0 {
		t.Fatalf("empty: OffsetOfStop = %v, want [0]", tt.OffsetOfStop)
	}

	if len(tt.TransferTarget) != 0 || len(tt.TransferModes) != 0 || len(tt.TransferWeights) != 0 {
		t.Fatalf("empty: expected no edges, got %d", len(tt.TransferTarget))
	}
}

func TestCalculateTransfersColocated(t *testing.T) {
	// Two distinct stops sharing an exact lat/lon: a symmetric pair of walk
	// edges, each with a zero weight.
	stops := []gtfs.GTFSStop{
		{GtfsId: "x", Name: "X", Lat: 37.7500, Lon: -122.4200},
		{GtfsId: "y", Name: "Y", Lat: 37.7500, Lon: -122.4200},
	}

	tt := CalculateTransfers(stops)

	wantOffsets := []uint32{0, 1, 2}
	if !equalUint32(tt.OffsetOfStop, wantOffsets) {
		t.Fatalf("OffsetOfStop = %v, want %v", tt.OffsetOfStop, wantOffsets)
	}

	if !equalStopID(tt.TransferTarget, []types.StopID{1, 0}) {
		t.Fatalf("TransferTarget = %v, want [1 0]", tt.TransferTarget)
	}

	for i, w := range tt.TransferWeights {
		if w.RealTime != 0 || w.PenalizedCost != 0 {
			t.Fatalf("TransferWeights[%d] = %+v, want zero", i, w)
		}

		if tt.TransferModes[i] != types.TransferModeWalk {
			t.Fatalf("TransferModes[%d] = %v, want walk", i, tt.TransferModes[i])
		}
	}

	assertSymmetric(t, tt)
}

func TestCalculateTransfersSingleStop(t *testing.T) {
	tt := CalculateTransfers([]gtfs.GTFSStop{
		{GtfsId: "solo", Name: "Solo", Lat: 37.75, Lon: -122.42},
	})

	if !equalUint32(tt.OffsetOfStop, []uint32{0, 0}) {
		t.Fatalf("OffsetOfStop = %v, want [0 0]", tt.OffsetOfStop)
	}

	if len(tt.TransferTarget) != 0 || len(tt.TransferModes) != 0 || len(tt.TransferWeights) != 0 {
		t.Fatalf("expected no edges, got %d", len(tt.TransferTarget))
	}
}

func TestCalculateTransfers(t *testing.T) {
	// s0..s2 strung north; s1 ~450 m from s0, s2 ~600 m from s0 (~150 m from s1);
	// s3 ~16 km away; s4 ~540 m due east of s0 — inside the k-d broad phase
	// (500 m * projectionSlack) but beyond the exact 500 m cut, pinning the
	// exact-cut rejection branch. Expected undirected edges: {s0,s1}, {s1,s2}.
	dLat450 := (450.0 / earthRadius) * 180 / math.Pi
	dLat600 := (600.0 / earthRadius) * 180 / math.Pi
	dLon540 := (540.0 / (earthRadius * math.Cos(degreesToRadians(37.7500)))) * 180 / math.Pi

	stops := []gtfs.GTFSStop{
		{GtfsId: "s0", Name: "S0", Lat: 37.7500, Lon: -122.4200},
		{GtfsId: "s1", Name: "S1", Lat: 37.7500 + dLat450, Lon: -122.4200},
		{GtfsId: "s2", Name: "S2", Lat: 37.7500 + dLat600, Lon: -122.4200},
		{GtfsId: "s3", Name: "S3", Lat: 37.9000, Lon: -122.4200},
		{GtfsId: "s4", Name: "S4", Lat: 37.7500, Lon: -122.4200 + dLon540},
	}

	tt := CalculateTransfers(stops)

	wantOffsets := []uint32{0, 1, 3, 4, 4, 4}
	if !equalUint32(tt.OffsetOfStop, wantOffsets) {
		t.Fatalf("OffsetOfStop = %v, want %v", tt.OffsetOfStop, wantOffsets)
	}

	wantTargets := []types.StopID{1, 0, 2, 1}
	if !equalStopID(tt.TransferTarget, wantTargets) {
		t.Fatalf("TransferTarget = %v, want %v", tt.TransferTarget, wantTargets)
	}

	for i, m := range tt.TransferModes {
		if m != types.TransferModeWalk {
			t.Fatalf("TransferModes[%d] = %v, want walk", i, m)
		}
	}

	if len(tt.TransferModes) != len(tt.TransferTarget) || len(tt.TransferWeights) != len(tt.TransferTarget) {
		t.Fatalf("column lengths differ: %d/%d/%d",
			len(tt.TransferTarget), len(tt.TransferModes), len(tt.TransferWeights))
	}

	// weight of edge s0->s1 == round(haversine / speed)
	d01 := haversineMeters(stops[0].Lat, stops[0].Lon, stops[1].Lat, stops[1].Lon)

	want01 := uint32(math.Round(d01 / WalkSpeedMetersPerSecond))
	if tt.TransferWeights[0].RealTime != want01 || tt.TransferWeights[0].PenalizedCost != want01 {
		t.Fatalf("weight s0->s1 = %+v, want RealTime=PenalizedCost=%d", tt.TransferWeights[0], want01)
	}

	// symmetry: every edge a->b has b->a with equal weight+mode
	assertSymmetric(t, tt)
}

func assertSymmetric(t *testing.T, tt *TransferTable) {
	t.Helper()

	type key struct {
		from, to types.StopID
	}

	seen := map[key]struct {
		mode   types.TransferMode
		weight types.DualWeight
	}{}

	for from := types.StopID(0); int(from)+1 < len(tt.OffsetOfStop); from++ {
		for i := tt.OffsetOfStop[from]; i < tt.OffsetOfStop[from+1]; i++ {
			seen[key{from, tt.TransferTarget[i]}] = struct {
				mode   types.TransferMode
				weight types.DualWeight
			}{tt.TransferModes[i], tt.TransferWeights[i]}
		}
	}

	for k, v := range seen {
		back, ok := seen[key{k.to, k.from}]
		if !ok {
			t.Fatalf("edge %d->%d has no reverse", k.from, k.to)
		}

		if back.mode != v.mode || back.weight != v.weight {
			t.Fatalf("edge %d<->%d asymmetric: %+v vs %+v", k.from, k.to, v, back)
		}
	}
}

func equalUint32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func equalStopID(a, b []types.StopID) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
