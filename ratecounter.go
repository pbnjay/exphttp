package exphttp

import (
	"strconv"
	"sync/atomic"
	"time"
)

// RateCounter is a thread-safe counter that allows you to count event rates
// over time with minimal memory overhead.
type RateCounter struct {
	others int64
	bins   []int64
	index  int
}

// NewCounter makes a new RateCounter that never rolls over, effectively a
// standard counter with a tiny bit of overhead.
func NewCounter() *RateCounter {
	return NewRateCounterWithGranularity(0, 1)
}

// NewRateCounter makes a new RateCounter using the interval provided and uses
// granularity = DefaultGranularity.
func NewRateCounter(interval time.Duration) *RateCounter {
	return NewRateCounterWithGranularity(interval, DefaultGranularity)
}

// NewRateCounterWithGranularity makes a new RateCounter using the interval
// and granularity settings provided. Granularity controls how accurate the
// rate is within an interval, at the expense of increased memory usage (one
// int64 per gran number of "buckets").
func NewRateCounterWithGranularity(interval time.Duration, gran int) *RateCounter {
	if interval <= time.Duration(0) || gran <= 1 {
		return &RateCounter{
			bins: []int64{0},
		}
	}

	r := &RateCounter{
		bins: make([]int64, gran),
	}

	go func() {
		i := 0
		t := time.NewTicker(interval / time.Duration(gran))
		for range t.C {
			i = r.index
			r.index = (r.index + 1) % gran
			r.others += r.bins[i] - atomic.SwapInt64(&r.bins[r.index], 0)
		}
	}()

	return r
}

// Add an even count into the RateCounter
func (r *RateCounter) Add(val int64) {
	atomic.AddInt64(&r.bins[r.index], val)
}

// Rate returns the current number of events in the last interval
func (r *RateCounter) Rate() int64 {
	return r.others + atomic.LoadInt64(&r.bins[r.index])
}

// String returns Rate() as a string (to implement expvar.Var)
func (r *RateCounter) String() string {
	return strconv.FormatInt(r.Rate(), 10)
}
