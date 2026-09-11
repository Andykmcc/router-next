package types

import "fmt"

type StopID uint32
type RouteID uint32

type Timestamp uint32 // seconds since midnight

const INFINITY Timestamp = Timestamp(^uint32(0) / 2)

type DualWeight struct {
	RealTime      uint32
	PenalizedCost uint32
}

// TransferMode identifies how a connecting (non-transit) leg is travelled.
// The zero value, TransferModeNone, means the leg is not a transfer at all
// (a transit leg, or an unpopulated Label).
type TransferMode uint8

const (
	TransferModeNone TransferMode = iota
	TransferModeWalk
)

func (m TransferMode) String() string {
	switch m {
	case TransferModeNone:
		return "transit"
	case TransferModeWalk:
		return "walk"
	default:
		return fmt.Sprintf("mode(%d)", uint8(m))
	}
}
