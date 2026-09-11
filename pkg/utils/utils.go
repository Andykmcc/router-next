package utils

import (
	"hash/fnv"
	"reflect"
	"router/pkg/types"
	"strconv"
	"strings"
)

// FNV64 returns the FNV-1a 64-bit hash of b as lowercase hex. Deterministic:
// same bytes in, same string out, on every platform.
func FNV64(b []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(b)

	return strconv.FormatUint(h.Sum64(), 16)
}

func Map[T any, U any](slice []T, f func(T) U) []U {
	result := make([]U, len(slice))
	for i, item := range slice {
		result[i] = f(item)
	}

	return result
}

func Reduce[T any, U any](slice []T, init U, f func(U, T) U) U {
	acc := init
	for _, v := range slice {
		acc = f(acc, v)
	}

	return acc
}

func SizeOf[T any]() int {
	return int(reflect.TypeFor[T]().Size())
}

const SnapshotLineWidth = 20

func SnapshotStr[T uint32 | types.StopID | types.TransferMode](arr []T) string {
	tokens := make([]string, len(arr))
	for i := range arr {
		tokens[i] = strconv.Itoa(int(arr[i]))
	}

	return snapshotRows(tokens)
}

func SnapshotDualWeights(arr []types.DualWeight) string {
	tokens := make([]string, len(arr))
	for i, w := range arr {
		tokens[i] = strconv.Itoa(int(w.RealTime)) + ":" + strconv.Itoa(int(w.PenalizedCost))
	}

	return snapshotRows(tokens)
}

// snapshotRows joins tokens into comma-separated rows of SnapshotLineWidth,
// rows separated by newlines, with no trailing newline. Empty input => "".
func snapshotRows(tokens []string) string {
	n := len(tokens)
	numRows := n / SnapshotLineWidth
	lastRowLength := n % SnapshotLineWidth

	if lastRowLength > 0 {
		numRows++
	}

	rows := make([]string, numRows)
	for rowIdx := range numRows {
		start := rowIdx * SnapshotLineWidth

		end := start + SnapshotLineWidth
		if rowIdx == numRows-1 && lastRowLength > 0 {
			end = start + lastRowLength
		}

		rows[rowIdx] = strings.Join(tokens[start:end], ",")
	}

	return strings.Join(rows, "\n")
}
