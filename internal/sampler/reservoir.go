package sampler

import "math/rand"

// reservoir implements Algorithm R: an unbiased sample of up to k items from
// a stream of unknown length, seen one at a time, without buffering the whole
// stream. Used to avoid biasing samples toward whichever page of results the
// visibility API happens to return first.
type reservoir[T any] struct {
	k     int
	items []T
	seen  int
	rng   *rand.Rand
}

func newReservoir[T any](k int, rng *rand.Rand) *reservoir[T] {
	return &reservoir[T]{k: k, rng: rng}
}

func (r *reservoir[T]) Add(item T) {
	r.seen++
	if len(r.items) < r.k {
		r.items = append(r.items, item)
		return
	}
	if j := r.rng.Intn(r.seen); j < r.k {
		r.items[j] = item
	}
}

func (r *reservoir[T]) Items() []T { return r.items }
