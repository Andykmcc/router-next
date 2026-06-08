package transfer

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/spatial/kdtree"

	"router/pkg/gtfs"
	"router/pkg/types"
)

type ComparableStop struct {
	gtfs.GTFSStop
}

const (
	TransferRadiusMeters    = 500.0
	metersPerDegreeLatitude = 111_132.0
	earthRadius             = 6_371_000
)

func (s ComparableStop) Compare(b kdtree.Comparable, d kdtree.Dim) float64 {
	t := b.(ComparableStop)

	switch d {
	case 0:
		return (s.Lat - t.Lat) * metersPerDegreeLatitude
	case 1:
		return (s.Lon - t.Lon) * metersPerDegreeLatitude * math.Cos(degreesToRadians((s.Lat+t.Lat)/2))
	}

	panic(fmt.Sprintf("invalid dimension: %d", d))
}

func (s ComparableStop) Dims() int { return 2 }

func (s ComparableStop) Distance(b kdtree.Comparable) float64 {
	t := b.(ComparableStop)
	return haversineMeters(s.Lat, s.Lon, t.Lat, t.Lon)
}

func CalculateTransfers(
	gtfsTransfers []gtfs.GTFSTransfer,
	gtfsStopIdMap map[gtfs.GTFSStopID]types.StopID,
) *TransferTable {
	numStops := len(gtfsStopIdMap)

	offsets := make([]uint32, numStops+1)

	return &TransferTable{
		OffsetOfStop:    offsets,
		TransferTarget:  make([]types.StopID, 0),
		TransferWeights: make([]types.DualWeight, 0),
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
