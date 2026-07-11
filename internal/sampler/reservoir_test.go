package sampler

import (
	"math/rand"
	"testing"
)

func TestReservoir_RespectsK(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	r := newReservoir[int](3, rng)
	for i := range 100 {
		r.Add(i)
	}
	if got := len(r.Items()); got != 3 {
		t.Fatalf("expected 3 items, got %d", got)
	}
}

func TestReservoir_FewerThanK(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	r := newReservoir[int](10, rng)
	for i := range 4 {
		r.Add(i)
	}
	if got := len(r.Items()); got != 4 {
		t.Fatalf("expected 4 items (fewer than k), got %d", got)
	}
}

func TestReservoir_ZeroK(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	r := newReservoir[int](0, rng)
	r.Add(1)
	r.Add(2)
	if got := len(r.Items()); got != 0 {
		t.Fatalf("expected 0 items for k=0, got %d", got)
	}
}

// TestReservoir_UnbiasedRoughly checks every item has roughly equal odds of
// selection across many trials — a coarse statistical sanity check, not a
// precise distribution test (that would be flaky by nature).
func TestReservoir_UnbiasedRoughly(t *testing.T) {
	const n, k, trials = 10, 3, 20000
	counts := make([]int, n)

	for trial := 0; trial < trials; trial++ {
		rng := rand.New(rand.NewSource(int64(trial)))
		r := newReservoir[int](k, rng)
		for i := range n {
			r.Add(i)
		}
		for _, item := range r.Items() {
			counts[item]++
		}
	}

	expected := float64(trials*k) / float64(n)
	for i, c := range counts {
		ratio := float64(c) / expected
		if ratio < 0.9 || ratio > 1.1 {
			t.Errorf("item %d selected %d times, expected ~%.0f (ratio %.2f) — reservoir looks biased", i, c, expected, ratio)
		}
	}
}
