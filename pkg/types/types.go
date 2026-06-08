package types

type StopID uint32
type RouteID uint32

type Timestamp uint32 // seconds since midnight

const INFINITY Timestamp = Timestamp(^uint32(0) / 2)

type DualWeight struct {
	RealTime      uint32
	PenalizedCost uint32
}
