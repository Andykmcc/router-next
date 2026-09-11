package utils

import (
	"router/pkg/types"
	"testing"
)

func TestSnapshotStr(t *testing.T) {
	// Test case 1
	lessThanLineWidth := make([]uint32, 6)

	formatted1 := SnapshotStr(lessThanLineWidth)
	expected1 := "0,0,0,0,0,0"
	if formatted1 != expected1 {
		t.Errorf("got:\n%s\nexpected:%s", formatted1, expected1)
	}

	// Test case 2
	moreThanLineWidth := make([]uint32, 44)

	formatted2 := SnapshotStr(moreThanLineWidth[:])
	expected2 :=
		`0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
0,0,0,0`
	if formatted2 != expected2 {
		t.Errorf("got:\n%s\nexpected:%s", formatted2, expected2)
	}
}

func TestFNV64(t *testing.T) {
	// Known FNV-1a 64-bit vectors.
	cases := map[string]string{
		"":    "cbf29ce484222325",
		"a":   "af63dc4c8601ec8c",
		"abc": "e71fa2190541574b",
	}
	for in, want := range cases {
		if got := FNV64([]byte(in)); got != want {
			t.Errorf("FNV64(%q) = %s, want %s", in, got, want)
		}
	}

	// Deterministic and sensitive to a single-byte change.
	if FNV64([]byte("abc")) != FNV64([]byte("abc")) {
		t.Error("FNV64 not deterministic")
	}

	if FNV64([]byte("abc")) == FNV64([]byte("abd")) {
		t.Error("FNV64 collided on a one-byte change")
	}
}

func TestSnapshotDualWeights(t *testing.T) {
	got := SnapshotDualWeights([]types.DualWeight{
		{RealTime: 1, PenalizedCost: 2},
		{RealTime: 30, PenalizedCost: 30},
		{RealTime: 0, PenalizedCost: 7},
	})
	want := "1:2,30:30,0:7"

	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
