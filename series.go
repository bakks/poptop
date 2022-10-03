package main

import "math"

type fifoSet struct {
	numValues int
	values    []float64
	sum       float64
}

func newFifoSet(numValues int) *fifoSet {
	return &fifoSet{
		values:    []float64{},
		numValues: numValues,
	}
}

func (this *fifoSet) AddValue(v float64) {
	vals := append(this.values, v)
	this.sum += v
	if len(vals) > this.numValues {
		this.sum -= vals[0]
		vals = vals[1:]
	}
	this.values = vals
}

func (this *fifoSet) Avg() float64 {
	return this.sum / float64(len(this.values))
}

type BoundedSeries struct {
	values    []float64 // array of values
	numValues int       // how many values have been requested to be stored
	maxValues int       // how many values we're actually storing (larger to allow smoothing)
	highWater int       // how many values have been populated
}

func NewBoundedSeries(numValues int) *BoundedSeries {
	maxValues := numValues * 2 // double the number of values to support moving averages
	values := make([]float64, maxValues)

	for i := 0; i < maxValues; i++ {
		values[i] = math.NaN()
	}

	return &BoundedSeries{
		values:    values,
		numValues: numValues,
		maxValues: maxValues,
		highWater: 0,
	}
}

func (this *BoundedSeries) AddValue(v float64) {
	if this.highWater < this.maxValues {
		this.values[this.highWater] = v
		this.highWater++
	} else {
		newValues := append(this.values, v)
		if len(newValues) > this.maxValues {
			newValues = newValues[len(newValues)-this.maxValues:]
		}
		this.values = newValues
	}
}

func (this *BoundedSeries) Values() []float64 {
	start := max(0, this.highWater-this.numValues)
	end := min(this.highWater, start+this.numValues)
	return this.values[start:end]
}

func (this *BoundedSeries) SmoothedValues(windowSize int) []float64 {
	if windowSize <= 1 {
		return this.Values()
	}

	start := max(0, this.highWater-this.numValues-windowSize)
	set := newFifoSet(windowSize)
	series := make([]float64, this.numValues)
	j := 0

	for i := start; i < this.highWater; i++ {
		set.AddValue(this.values[i])
		if i >= this.highWater-this.numValues {
			series[j] = set.Avg()
			j++
		}
	}

	for ; j < len(series); j++ {
		series[j] = math.NaN()
	}

	return series
}
