package transfer

import (
	"router/pkg/types"
)

type TransferTable struct {
	OffsetOfStop    []uint32
	TransferTarget  []types.StopID
	TransferWeights []types.DualWeight
}
