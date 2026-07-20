package baserpc

import "testing"

// The public Base RPC eth_getLogs cap is exactly 10,000 blocks inclusive; a
// span of 10,001 is rejected with -32614 and, in the Free-tier + catch-up case,
// sticks the base_rpc circuit breaker open. Every generated chunk must stay at
// or under the cap.
func TestBlockRanges_NeverExceedsCap(t *testing.T) {
	const maxRange = uint64(10000)
	ranges := blockRanges(1_000_000, 1_045_000, maxRange)
	if len(ranges) == 0 {
		t.Fatal("no ranges generated")
	}

	// Coverage: the chunks span exactly the requested inclusive range.
	if ranges[0][0] != 1_000_000 {
		t.Fatalf("first start = %d, want 1000000", ranges[0][0])
	}
	if last := ranges[len(ranges)-1][1]; last != 1_045_000 {
		t.Fatalf("last end = %d, want 1045000", last)
	}

	var sawFullChunk bool
	for i, r := range ranges {
		width := r[1] - r[0] + 1 // inclusive span
		if width > maxRange {
			t.Fatalf("chunk %d spans %d blocks (%d..%d), exceeds the %d-block cap", i, width, r[0], r[1], maxRange)
		}
		if width == maxRange {
			sawFullChunk = true
		}
		if i > 0 {
			if prevEnd := ranges[i-1][1]; r[0] != prevEnd+1 {
				t.Fatalf("chunk %d starts at %d, want %d (gap or overlap)", i, r[0], prevEnd+1)
			}
		}
	}
	if !sawFullChunk {
		t.Fatal("expected at least one full 10000-block chunk")
	}
}

// Negative control for the off-by-one that caused the outage: a full inclusive
// 10000-block span is one chunk; adding a single block forces a second chunk,
// never a 10001-block span.
func TestBlockRanges_ExactBoundary(t *testing.T) {
	const maxRange = uint64(10000)

	// [0, 9999] is exactly 10000 blocks inclusive: a single chunk.
	one := blockRanges(0, 9999, maxRange)
	if len(one) != 1 || one[0] != [2]uint64{0, 9999} {
		t.Fatalf("blockRanges(0,9999) = %v, want a single chunk [0,9999]", one)
	}

	// [0, 10000] is 10001 blocks: must split into two chunks, NOT emit one
	// 10001-block span (which is what the pre-fix start+maxRange formula did).
	two := blockRanges(0, 10000, maxRange)
	if len(two) != 2 || two[0] != [2]uint64{0, 9999} || two[1] != [2]uint64{10000, 10000} {
		t.Fatalf("blockRanges(0,10000) = %v, want [[0,9999],[10000,10000]]", two)
	}
}

func TestBlockRanges_Empty(t *testing.T) {
	if r := blockRanges(100, 50, 10000); len(r) != 0 {
		t.Fatalf("start>latest should yield no ranges, got %v", r)
	}
	if r := blockRanges(0, 100, 0); len(r) != 0 {
		t.Fatalf("zero maxRange should yield no ranges, got %v", r)
	}
}
