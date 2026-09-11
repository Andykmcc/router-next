package transfer

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"gonum.org/v1/gonum/spatial/kdtree"

	"router/pkg/gtfs"
	"router/pkg/types"
)

const (
	TransferRadiusMeters     = 500.0
	earthRadius              = 6_371_000
	WalkSpeedMetersPerSecond = 1.33

	// projectionSlack widens the broad-phase k-d radius to absorb equirectangular
	// x-axis distortion before the exact haversine cut.
	projectionSlack = 1.15
)

type projection struct {
	lat0 float64
	lon0 float64
}

func newProjection(stops []gtfs.GTFSStop) projection {
	if len(stops) == 0 {
		return projection{lat0: 0, lon0: 0}
	}

	var sumLat, sumLon float64
	for _, s := range stops {
		sumLat += s.Lat
		sumLon += s.Lon
	}

	n := float64(len(stops))

	return projection{lat0: sumLat / n, lon0: sumLon / n}
}

// project maps a lat/lon (degrees) to local equirectangular metres about the
// projection's reference point. Only the x axis carries any distortion (the
// cos(lat0) factor); across a metro-area feed that is ~1-2%.
func (p projection) project(lat, lon float64) (x, y float64) {
	x = earthRadius * degreesToRadians(lon-p.lon0) * math.Cos(degreesToRadians(p.lat0))
	y = earthRadius * degreesToRadians(lat-p.lat0)

	return x, y
}

// ComparableStop is a stop positioned in the local equirectangular metre frame,
// satisfying gonum's kdtree.Comparable with a consistent Euclidean metric.
type ComparableStop struct {
	gtfs.GTFSStop

	ID types.StopID
	X  float64
	Y  float64
}

func (s ComparableStop) Compare(b kdtree.Comparable, d kdtree.Dim) float64 {
	t := b.(ComparableStop)

	switch d {
	case 0:
		return s.X - t.X
	case 1:
		return s.Y - t.Y
	}

	panic(fmt.Sprintf("invalid dimension: %d", d))
}

func (s ComparableStop) Dims() int { return 2 }

func (s ComparableStop) Distance(b kdtree.Comparable) float64 {
	t := b.(ComparableStop)
	dx := s.X - t.X
	dy := s.Y - t.Y

	return dx*dx + dy*dy
}

type comparableStops []ComparableStop

func (cs comparableStops) Index(i int) kdtree.Comparable         { return cs[i] }
func (cs comparableStops) Len() int                              { return len(cs) }
func (cs comparableStops) Slice(start, end int) kdtree.Interface { return cs[start:end] }

func (cs comparableStops) Pivot(d kdtree.Dim) int {
	p := comparableStopPlane{stops: cs, dim: d}

	return kdtree.Partition(p, kdtree.MedianOfMedians(p))
}

type comparableStopPlane struct {
	stops comparableStops
	dim   kdtree.Dim
}

func (p comparableStopPlane) Len() int { return len(p.stops) }

func (p comparableStopPlane) Less(i, j int) bool {
	if p.dim == 0 {
		return p.stops[i].X < p.stops[j].X
	}

	return p.stops[i].Y < p.stops[j].Y
}

func (p comparableStopPlane) Swap(i, j int) { p.stops[i], p.stops[j] = p.stops[j], p.stops[i] }

func (p comparableStopPlane) Slice(start, end int) kdtree.SortSlicer {
	p.stops = p.stops[start:end]

	return p
}

type transferEdge struct {
	target types.StopID
	mode   types.TransferMode
	weight types.DualWeight
}

// CalculateTransfers builds the CSR transfer table: every ordered pair of
// distinct stops within TransferRadiusMeters gets a walk edge. It is the
// mode-agnostic merge point — future modes add a builder and merge here.
func CalculateTransfers(stops []gtfs.GTFSStop) *TransferTable {
	if len(stops) == 0 {
		return &TransferTable{
			OffsetOfStop:    make([]uint32, 1),
			TransferTarget:  nil,
			TransferModes:   nil,
			TransferWeights: nil,
		}
	}

	adjacency := walkTransfers(stops)

	return flattenTransfers(len(stops), adjacency)
}

func walkTransfers(stops []gtfs.GTFSStop) [][]transferEdge {
	proj := newProjection(stops)

	points := make(comparableStops, len(stops))
	for i, s := range stops {
		x, y := proj.project(s.Lat, s.Lon)
		points[i] = ComparableStop{GTFSStop: s, ID: types.StopID(i), X: x, Y: y}
	}

	// kdtree.New reorders points in place; that is fine, every point is still
	// visited once below and edges are keyed by ComparableStop.ID.
	tree := kdtree.New(points, false)

	broad := TransferRadiusMeters * projectionSlack

	adjacency := make([][]transferEdge, len(stops))

	for _, q := range points {
		keeper := kdtree.NewDistKeeper(broad * broad)
		tree.NearestSet(keeper, q)

		for _, cd := range keeper.Heap {
			if cd.Comparable == nil {
				continue
			}

			cand := cd.Comparable.(ComparableStop)
			if cand.ID == q.ID {
				continue
			}

			dist := haversineMeters(q.Lat, q.Lon, cand.Lat, cand.Lon)
			if dist > TransferRadiusMeters {
				continue
			}

			secs := uint32(math.Round(dist / WalkSpeedMetersPerSecond))
			adjacency[q.ID] = append(adjacency[q.ID], transferEdge{
				target: cand.ID,
				mode:   types.TransferModeWalk,
				weight: types.DualWeight{RealTime: secs, PenalizedCost: secs},
			})
		}
	}

	return adjacency
}

func flattenTransfers(numStops int, adjacency [][]transferEdge) *TransferTable {
	offsets := make([]uint32, numStops+1)

	var (
		targets []types.StopID
		modes   []types.TransferMode
		weights []types.DualWeight
	)

	for s := range numStops {
		edges := adjacency[s]
		// Sort key (target, mode) is unique per edge today, so SortFunc's
		// instability is harmless. A second edge to the same (target, mode)
		// would need a stable tie-break.
		slices.SortFunc(edges, func(a, b transferEdge) int {
			if a.target != b.target {
				return cmp.Compare(a.target, b.target)
			}

			return cmp.Compare(a.mode, b.mode)
		})

		for _, e := range edges {
			targets = append(targets, e.target)
			modes = append(modes, e.mode)
			weights = append(weights, e.weight)
		}

		offsets[s+1] = uint32(len(targets))
	}

	return &TransferTable{
		OffsetOfStop:    offsets,
		TransferTarget:  targets,
		TransferModes:   modes,
		TransferWeights: weights,
	}
}

func degreesToRadians(d float64) float64 {
	return d * math.Pi / 180
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	lat1R := degreesToRadians(lat1)
	lat2R := degreesToRadians(lat2)
	dLat := lat2R - lat1R
	dLon := degreesToRadians(lon2 - lon1)

	havLat := math.Pow(math.Sin(dLat/2), 2)
	havLon := math.Pow(math.Sin(dLon/2), 2)

	havTheta := havLat + math.Cos((lat1R))*math.Cos(lat2R)*havLon
	theta := 2 * math.Atan2(math.Sqrt(havTheta), math.Sqrt(1-havTheta))

	return earthRadius * theta
}
