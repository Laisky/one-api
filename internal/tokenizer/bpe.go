package tiktoken

import (
	"math"
)

// checkedMergePartCount returns the sentinel-inclusive table length without overflowing int.
// Invalid dimensions panic before arithmetic, matching the original allocation failure
// class without introducing a smaller limit for representable inputs.
func checkedMergePartCount(pieceBytes int) int {
	if pieceBytes < 0 || pieceBytes >= math.MaxInt {
		panic("tokenizer: BPE part count exceeds int range")
	}
	return pieceBytes + 1
}

// bytePairMerge applies the pinned upstream merge algorithm to piece and returns f's values.
func bytePairMerge[T any](piece []byte, ranks map[string]int, f func(start, end int) T) []T {
	parts := make([][2]int, checkedMergePartCount(len(piece)))
	for i := 0; i < len(parts); i++ {
		parts[i][0], parts[i][1] = i, math.MaxInt // use max int as sentinel
	}

	getRank := func(startIdx, skip int) int {
		if startIdx+skip+2 < len(parts) {
			b := piece[parts[startIdx][0]:parts[startIdx+skip+2][0]]
			rank, ok := ranks[string(b)]
			if ok {
				return rank
			}
		}
		return -1 // use -1 to represent None
	}

	for i := 0; i < len(parts)-2; i++ {
		if rank := getRank(i, 0); rank >= 0 {
			parts[i][1] = rank
		}
	}

	for len(parts) > 1 {
		minRank, minIdx := math.MaxInt, -1
		for i := 0; i < len(parts)-1; i++ {
			if parts[i][1] < minRank {
				minRank, minIdx = parts[i][1], i
			}
		}

		if minRank < math.MaxInt {
			i := minIdx
			rank := getRank(i, 1)
			if rank >= 0 {
				parts[i][1] = rank
			} else {
				parts[i][1] = math.MaxInt
			}
			if i > 0 {
				rk := getRank(i-1, 1)
				if rk >= 0 {
					parts[i-1][1] = rk
				} else {
					parts[i-1][1] = math.MaxInt
				}
			}
			parts = append(parts[:i+1], parts[i+2:]...)
		} else {
			break
		}
	}

	out := make([]T, len(parts)-1)
	for i := 0; i < len(out); i++ {
		out[i] = f(parts[i][0], parts[i+1][0])
	}
	return out
}

// bytePairEncode returns the original rank sequence for one pre-tokenized piece.
func bytePairEncode(piece []byte, ranks map[string]int) []int {
	if len(piece) == 1 {
		v := ranks[string(piece)]
		return []int{v}
	}
	return bytePairMerge(piece, ranks, func(start, end int) int {
		return ranks[string(piece[start:end])]
	})
}
