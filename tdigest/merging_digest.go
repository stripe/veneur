// Package tdigest provides an implementation of Ted Dunning's t-digest, an
// approximate histogram for online, distributed applications. For more details,
// refer to Dunning's paper and the reference implementations.
//
// https://github.com/tdunning/t-digest/blob/master/docs/t-digest-paper/histo.pdf
//
// https://github.com/tdunning/t-digest/blob/master/src/main/java/com/tdunning/math/stats/
package tdigest

import (
	"bytes"
	"encoding/gob"
	"math"
	"math/rand"
	"sort"
)

// A t-digest using the merging implementation. MergingDigest is not safe for
// use by multiple goroutines simultaneously, and its methods must not be
// invoked concurrently (including Quantile and CDF).
type MergingDigest struct {
	compression float64

	// main list of centroids
	mainCentroids []centroid
	// total weight of unmerged centroids
	mainWeight float64

	// centroids that have been added but not yet merged into main list
	tempCentroids []centroid
	// total weight of unmerged centroids
	tempWeight float64

	// an extra list used during merging and then swapped with the main list
	// TODO: there must be an in-place merge we can do to eliminate this
	// outside of mergeAllTemps(), this is always empty
	mergedCentroids []centroid

	min float64
	max float64
}

// fields must be exported to allow encoding
type centroid struct {
	Mean   float64
	Weight float64
}

var _ sort.Interface = centroidList{}

// sort centroids by their mean
type centroidList []centroid

func (cl centroidList) Len() int {
	return len(cl)
}
func (cl centroidList) Less(i, j int) bool {
	return cl[i].Mean < cl[j].Mean
}
func (cl centroidList) Swap(i, j int) {
	cl[i], cl[j] = cl[j], cl[i]
}

// Initializes a new merging t-digest using the given compression parameter.
// Lower compression values result in reduced memory consumption and less
// precision, especially at the median. Values from 20 to 1000 are recommended
// in Dunning's paper.
func NewMerging(compression float64) *MergingDigest {
	// this is a provable upper bound on the size of the centroid list
	// TODO: derive it myself
	sizeBound := int((math.Pi * compression / 2) + 0.5)

	return &MergingDigest{
		compression:     compression,
		mainCentroids:   make([]centroid, 0, sizeBound),
		mergedCentroids: make([]centroid, 0, sizeBound),
		tempCentroids:   make([]centroid, 0, estimateTempBuffer(compression)),
		min:             math.Inf(+1),
		max:             math.Inf(-1),
	}
}

func estimateTempBuffer(compression float64) int {
	// this heuristic comes from Dunning's paper
	// 925 is the maximum point of this quadratic equation
	// TODO: let's derive and justify this heuristic
	tempCompression := math.Min(925, math.Max(20, compression))
	return int(7.5 + 0.37*tempCompression - 2e-4*tempCompression*tempCompression)
}

// Adds a new value to the t-digest, with a given weight that must be positive.
// Infinities and NaN cannot be added.
func (td *MergingDigest) Add(value float64, weight float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) || weight <= 0 {
		panic("invalid value added")
	}

	if len(td.tempCentroids) == cap(td.tempCentroids) {
		td.mergeAllTemps()
	}

	td.min = math.Min(td.min, value)
	td.max = math.Max(td.max, value)

	td.tempCentroids = append(td.tempCentroids, centroid{value, weight})
	td.tempWeight += weight
}

// combine the mainCentroids and tempCentroids into mergedCentroids, then swap
// mergedCentroids and mainCentroids
func (td *MergingDigest) mergeAllTemps() {
	// this optimization is really important! if you remove it, the main list
	// will get merged into itself every time this is called
	if len(td.tempCentroids) == 0 {
		return
	}

	// we iterate over both centroid lists from least to greatest mean, so first
	// we have to sort this one
	sort.Sort(centroidList(td.tempCentroids))
	tempIndex := 0
	mainIndex := 0

	// total weight that the final t-digest will have, after everything is merged
	totalWeight := td.mainWeight + td.tempWeight
	// how much weight has been merged so far
	mergedWeight := 0.0
	// the index of the last quantile to be merged into the previous centroid
	// this value gets updated each time we split a new centroid out instead of
	// merging into the current one
	lastMergedIndex := 0.0

	for tempIndex < len(td.tempCentroids) || mainIndex < len(td.mainCentroids) {
		// if the main list is exhausted
		if mainIndex == len(td.mainCentroids) ||
			// or if both lists are unexhausted and temp's current
			// centroid is lower than main's current centroid
			(tempIndex < len(td.tempCentroids) && td.tempCentroids[tempIndex].Mean <= td.mainCentroids[mainIndex].Mean) {
			// then merge the temp centroid into the previous one
			lastMergedIndex = td.mergeOne(mergedWeight, totalWeight, lastMergedIndex, td.tempCentroids[tempIndex])
			mergedWeight += td.tempCentroids[tempIndex].Weight
			tempIndex++
		} else {
			// either the temp list is exhausted, or the current temp centroid
			// has greater mean than the current main one
			// we merge the current main centroid into the previous main centroid
			lastMergedIndex = td.mergeOne(mergedWeight, totalWeight, lastMergedIndex, td.mainCentroids[mainIndex])
			mergedWeight += td.mainCentroids[mainIndex].Weight
			mainIndex++
		}
	}

	// we've merged all elements from tempCentroids and mainCentroids into
	// mergedCentroids, so now we can swap in the new values
	td.mainCentroids, td.mergedCentroids = td.mergedCentroids, td.mainCentroids
	td.mainWeight = totalWeight
	// empty the other lists
	td.tempCentroids = td.tempCentroids[:0]
	td.tempWeight = 0
	td.mergedCentroids = td.mergedCentroids[:0]
}

// merges a single centroid into the mergedCentroids list
// note that "merging" sometimes creates a new centroid in the list, however
// the length of the list has a strict upper bound (see constructor)
func (td *MergingDigest) mergeOne(beforeWeight, totalWeight, beforeIndex float64, next centroid) float64 {
	// compute the quantile index of the element we're about to merge
	nextIndex := td.indexEstimate((beforeWeight + next.Weight) / totalWeight)

	if nextIndex-beforeIndex > 1 || len(td.mergedCentroids) == 0 {
		// the new index is far away from the last index of the current centroid
		// therefore we cannot merge into the current centroid or it would
		// become too wide, so we will append a new centroid
		td.mergedCentroids = append(td.mergedCentroids, next)
		// return the last index that was merged into the previous centroid
		return td.indexEstimate(beforeWeight / totalWeight)
	} else {
		// the new index fits into the range of the current centroid, so we
		// combine it into the current centroid's values
		// this computation is known as welford's method, the order matters
		// weight must be updated before mean
		td.mergedCentroids[len(td.mergedCentroids)-1].Weight += next.Weight
		td.mergedCentroids[len(td.mergedCentroids)-1].Mean += (next.Mean - td.mergedCentroids[len(td.mergedCentroids)-1].Mean) * next.Weight / td.mergedCentroids[len(td.mergedCentroids)-1].Weight

		// we did not create a new centroid, so the trailing index of the previous
		// centroid remains
		return beforeIndex
	}
}

// given a quantile, estimate the index of the centroid that contains it using
// the given compression
func (td *MergingDigest) indexEstimate(quantile float64) float64 {
	// TODO: a polynomial approximation of arcsine should be a lot faster
	return td.compression * ((math.Asin(2*quantile-1) / math.Pi) + 0.5)
}

// Returns the approximate percentage of values in td that are below value (ie
// the cumulative distribution function). Returns NaN if the digest is empty.
func (td *MergingDigest) CDF(value float64) float64 {
	td.mergeAllTemps()

	if len(td.mainCentroids) == 0 {
		return math.NaN()
	}
	if value <= td.min {
		return 0
	}
	if value >= td.max {
		return 1
	}

	weightSoFar := 0.0
	lowerBound := td.min
	for i, c := range td.mainCentroids {
		upperBound := td.centroidUpperBound(i)
		if value < upperBound {
			// the value falls inside the bounds of this centroid
			// based on the assumed uniform distribution, we calculate how much
			// of this centroid's weight is below the value
			weightSoFar += c.Weight * (value - lowerBound) / (upperBound - lowerBound)
			return weightSoFar / td.mainWeight
		}

		// the value is above this centroid, so sum the weight and carry on
		weightSoFar += c.Weight
		lowerBound = upperBound
	}

	// should never be reached, since the final loop comparison is value < td.max
	return math.NaN()
}

// Returns a value such that the fraction of values in td below that value is
// approximately equal to quantile. Returns NaN if the digest is empty.
func (td *MergingDigest) Quantile(quantile float64) float64 {
	if quantile < 0 || quantile > 1 {
		panic("quantile out of bounds")
	}
	td.mergeAllTemps()

	// add up the weights of centroids in ascending order until we reach a
	// centroid that pushes us over the quantile
	q := quantile * td.mainWeight
	weightSoFar := 0.0
	lowerBound := td.min
	for i, c := range td.mainCentroids {
		upperBound := td.centroidUpperBound(i)
		if q <= weightSoFar+c.Weight {
			// the target quantile is somewhere inside this centroid
			// we compute how much of this centroid's weight falls into the quantile
			proportion := (q - weightSoFar) / c.Weight
			// and interpolate what value that corresponds to inside a uniform
			// distribution
			return lowerBound + (proportion * (upperBound - lowerBound))
		}

		// the quantile is above this centroid, so sum the weight and carry on
		weightSoFar += c.Weight
		lowerBound = upperBound
	}

	// should never be reached unless empty, since the final comparison is
	// q <= td.mainWeight
	return math.NaN()
}

func (td *MergingDigest) Min() float64 {
	return td.min
}
func (td *MergingDigest) Max() float64 {
	return td.max
}
func (td *MergingDigest) Count() float64 {
	return td.mainWeight + td.tempWeight
}

// we assume each centroid contains a uniform distribution of values
// the lower bound of the distribution is the midpoint between this centroid and
// the previous one (or the minimum, if this is the lowest centroid)
// similarly, the upper bound is the midpoint between this centroid and the
// next one (or the maximum, if this is the greatest centroid)
// this function returns the position of the upper bound (the lower bound is
// equal to the upper bound of the previous centroid)
// this assumption is justified empirically in dunning's paper
// TODO: does this assumption actually apply to our implementation?
func (td *MergingDigest) centroidUpperBound(i int) float64 {
	if i != len(td.mainCentroids)-1 {
		return (td.mainCentroids[i+1].Mean + td.mainCentroids[i].Mean) / 2
	} else {
		return td.max
	}
}

// Merge another digest into this one. This does not allocate any memory or
// modify other; however neither td nor other can be shared concurrently during
// the execution of this method.
func (td *MergingDigest) Merge(other *MergingDigest) {
	// we want to add each centroid from other.mainCentroids to this one,
	// preferably in random order
	// we don't want to allocate any memory (that's too expensive), nor do we
	// want to shuffle other.mainCentroids (that's destructive and would force
	// us to sort it afterwards)
	// we can use other.mergeCentroids as a scratch buffer instead and
	// perform the entire operation in O(len(other.mainCentroids)) with zero
	// allocations
	other.mergedCentroids = other.mergedCentroids[:len(other.mainCentroids)]

	// fisher-yates shuffle from mainCentroids into mergedCentroids
	for i := range other.mainCentroids {
		j := rand.Intn(i + 1)
		other.mergedCentroids[i] = other.mergedCentroids[j]
		other.mergedCentroids[j] = other.mainCentroids[i]
	}

	for i := range other.mergedCentroids {
		td.Add(other.mergedCentroids[i].Mean, other.mergedCentroids[i].Weight)
	}
	other.mergedCentroids = other.mergedCentroids[:0]

	// we did not merge other's temps, so we need to add those too
	// they're unsorted so there's no need to shuffle them
	for i := range other.tempCentroids {
		td.Add(other.tempCentroids[i].Mean, other.tempCentroids[i].Weight)
	}
}

var _ gob.GobEncoder = &MergingDigest{}
var _ gob.GobDecoder = &MergingDigest{}

func (td *MergingDigest) GobEncode() ([]byte, error) {
	td.mergeAllTemps()

	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)

	if err := enc.Encode(td.mainCentroids); err != nil {
		return nil, err
	}
	if err := enc.Encode(td.compression); err != nil {
		return nil, err
	}
	if err := enc.Encode(td.min); err != nil {
		return nil, err
	}
	if err := enc.Encode(td.max); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (td *MergingDigest) GobDecode(b []byte) error {
	dec := gob.NewDecoder(bytes.NewReader(b))

	if err := dec.Decode(&td.mainCentroids); err != nil {
		return err
	}
	if err := dec.Decode(&td.compression); err != nil {
		return err
	}
	if err := dec.Decode(&td.min); err != nil {
		return err
	}
	if err := dec.Decode(&td.max); err != nil {
		return err
	}

	// reinitialize the remaining variables
	td.mainWeight = 0
	for _, c := range td.mainCentroids {
		td.mainWeight += c.Weight
	}
	td.tempWeight = 0
	// avoid reallocating if the compressions are the same
	if cap(td.mergedCentroids) != cap(td.mainCentroids) {
		td.mergedCentroids = make([]centroid, 0, cap(td.mainCentroids))
	}
	if tempSize := estimateTempBuffer(td.compression); cap(td.tempCentroids) != tempSize {
		td.tempCentroids = make([]centroid, 0, tempSize)
	} else {
		// discard any unmerged centroids if we didn't reallocate
		td.tempCentroids = td.tempCentroids[:0]
	}

	return nil
}
