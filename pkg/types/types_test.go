package types_test

import (
	"testing"

	"router/pkg/types"
)

func TestTransferModeString(t *testing.T) {
	cases := map[types.TransferMode]string{
		types.TransferModeNone:  "transit",
		types.TransferModeWalk:  "walk",
		types.TransferMode(200): "mode(200)",
	}

	for mode, want := range cases {
		if got := mode.String(); got != want {
			t.Errorf("TransferMode(%d).String() = %q, want %q", uint8(mode), got, want)
		}
	}
}
