package transfer

import (
	"router/pkg/gtfs"
	"router/pkg/types"
)

func CalculateTransfers(gtfsTable gtfs.GTFSTable) *TransferTable {
	offsets := make([]uint32, len(gtfsTable.Stops)+1)

	return &TransferTable{
		OffsetOfStop:    offsets,
		TransferTarget:  make([]types.StopID, 0),
		TransferWeights: make([]types.DualWeight, 0),
	}
}
