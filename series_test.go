package main

import (
	"math"
	"runtime/debug"
	"testing"
)

const float64EqualityThreshold = 1e-9

func floatEq(a, b float64) bool {
	return math.Abs(a-b) <= float64EqualityThreshold
}

func assertEq(t *testing.T, a, b float64) {
	if math.IsNaN(a) && math.IsNaN(b) {
		return
	}
	if !floatEq(a, b) {
		t.Errorf("Not equal: %f != %f", a, b)
		t.Log(string(debug.Stack()))
	}
}

func assertSliceEq(t *testing.T, a, b []float64) {
	if len(a) != len(b) {
		t.Errorf("Unequal slice lengths: %d != %d", len(a), len(b))
		t.Log(string(debug.Stack()))
		return
	}

	for i := 0; i < len(a); i++ {
		assertEq(t, a[i], b[i])
	}
}

func TestBoundedSeriesSmoothing(t *testing.T) {
	series := NewBoundedSeries(5)

	series.AddValue(0)
	assertSliceEq(t, series.Values(), []float64{0})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0, math.NaN(), math.NaN(), math.NaN(), math.NaN()})

	series.AddValue(1)
	assertSliceEq(t, series.Values(), []float64{0, 1})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0, 0.5, math.NaN(), math.NaN(), math.NaN()})

	series.AddValue(2)
	assertSliceEq(t, series.Values(), []float64{0, 1, 2})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0, 0.5, 1, math.NaN(), math.NaN()})

	series.AddValue(3)
	assertSliceEq(t, series.Values(), []float64{0, 1, 2, 3})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0, 0.5, 1, 2, math.NaN()})

	series.AddValue(4)
	assertSliceEq(t, series.Values(), []float64{0, 1, 2, 3, 4})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0, 0.5, 1, 2, 3})

	series.AddValue(5)
	assertSliceEq(t, series.Values(), []float64{1, 2, 3, 4, 5})
	assertSliceEq(t, series.SmoothedValues(3), []float64{0.5, 1, 2, 3, 4})

	series.AddValue(6)
	assertSliceEq(t, series.Values(), []float64{2, 3, 4, 5, 6})
	assertSliceEq(t, series.SmoothedValues(1), []float64{2, 3, 4, 5, 6})
	assertSliceEq(t, series.SmoothedValues(3), []float64{1, 2, 3, 4, 5})
}
