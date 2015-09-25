package exphttp

import (
	"strconv"
	"sync/atomic"
	"time"
)

// MovingAverage is a thread-safe moving average tracker that uses minimal
// memory overhead.
type MovingAverage struct {
	otherSums   int64
	otherCounts int64
	sums        []int64
	counts      []int64
	index       int
}

// NewAverage makes a new MovingAverage that never rolls over,
// effectively a standard average counter.
func NewAverage() *MovingAverage {
	return NewMovingAverageWithGranularity(0, 1)
}

// NewMovingAverage makes a new MovingAverage using the interval
// provided and DefaultGranularity.
func NewMovingAverage(interval time.Duration) *MovingAverage {
	return NewMovingAverageWithGranularity(interval, DefaultGranularity)
}

// NewMovingAverageWithGranularity makes a new MovingAverage
// using the interval and granularity settings provided. Granularity controls
// how accurate the moving average is within an interval, at the expense of
// increased memory usage (two int64 per gran number of "buckets").
func NewMovingAverageWithGranularity(interval time.Duration, gran int) *MovingAverage {
	if interval <= time.Duration(0) || gran <= 1 {
		return &MovingAverage{
			sums:   []int64{0},
			counts: []int64{0},
		}
	}

	r := &MovingAverage{
		sums:   make([]int64, gran),
		counts: make([]int64, gran),
	}

	go func() {
		i := 0
		t := time.NewTicker(interval / time.Duration(gran))
		for range t.C {
			i = r.index
			r.index = (r.index + 1) % gran

			// this is "as atomic" as easily possible...
			s := atomic.SwapInt64(&r.sums[r.index], 0)
			n := atomic.SwapInt64(&r.counts[r.index], 0)
			r.otherSums += r.sums[i] - s
			r.otherCounts += r.counts[i] - n
		}
	}()

	return r
}

// Add an event count into the MovingAverage
func (r *MovingAverage) Add(val int64) {
	atomic.AddInt64(&r.sums[r.index], val)
	atomic.AddInt64(&r.counts[r.index], 1)
}

// Average returns the average number of events in the last interval
func (r *MovingAverage) Average() int64 {
	// this is "as atomic" as easily possible...
	s := atomic.LoadInt64(&r.sums[r.index])
	n := atomic.LoadInt64(&r.counts[r.index])
	s += r.otherSums
	n += r.otherCounts
	if n == 0 {
		return 0
	}
	return s / n
}

// String returns Average() as a string (to implement expvar.Var)
func (r *MovingAverage) String() string {
	return strconv.FormatInt(r.Average(), 10)
}
