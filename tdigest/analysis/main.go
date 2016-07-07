package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/stripe/veneur/tdigest"
)

// populate a single t-digest, of a given compression, with a given number of
// samples, drawn from the given distribution function
// then writes various statistics to the given CSVs
func runOnce(distribution func() float64, compression float64, samples int, distname string, run int, deviations, centroidErrors, errors, sizes *csv.Writer) {
	td := tdigest.NewMerging(compression, true)

	allSamples := make([]float64, samples)
	for i := 0; i < samples; i++ {
		sample := distribution()
		td.Add(sample, 1)
		allSamples[i] = sample
	}
	sort.Float64s(allSamples)

	centroids := td.Centroids()
	for i, centroid := range centroids {
		// compute the approximate cdf for this centroid's approximate mean
		// this is roughly equivalent to the sum of all previous centroids'
		// weights, plus half this centroid's weight, divided by the total weight
		// https://github.com/tdunning/t-digest/blob/master/src/test/java/com/tdunning/math/stats/TDigestTest.java#L357
		thisCDF := td.CDF(centroid.Mean)

		// compute the cdf of the centroid's approximate mean, but over the real sample set
		realCDF := floatCDF(allSamples, centroid.Mean)

		// find the real sample that matches this centroid's approximate cdf
		// this should be close to the centroid's real mean
		realMean := floatQuantile(allSamples, thisCDF)

		// compute distances to previous and next centroids (ie the range
		// that this centroid is expected to cover)
		distanceToPrev := centroid.Mean - td.Min()
		if i > 0 {
			distanceToPrev = centroid.Mean - centroids[i-1].Mean
		}
		distanceToNext := td.Max() - centroid.Mean
		if i < len(centroids)-1 {
			distanceToNext = centroids[i+1].Mean - centroid.Mean
		}

		// compute the centroid's real mean using its sample set
		sampledMean := 0.0
		for _, sample := range centroid.Samples {
			sampledMean += sample
			// equivalent to deviations.csv from dunning's tests
			deviations.Write(stringifySlice(
				distname,
				run,
				thisCDF,
				centroid.Weight,
				sample,
				centroid.Mean,
				distanceToPrev,
				distanceToNext,
				// where is this sample, as a proportion of the range covered by its centroid?
				(sample-centroid.Mean)/(distanceToNext+distanceToPrev),
			))
		}
		sampledMean /= float64(len(centroid.Samples))
		// and compute the CDF corresopnding to this value
		sampledCDF := floatCDF(allSamples, sampledMean)

		// this csv is equivalent to errors.csv from dunning's tests, but
		// instead of testing a fixed range of quantiles, we test every centroid
		centroidErrors.Write(stringifySlice(
			distname,
			run,
			centroid.Mean,
			realMean, // this column is equivalent to the quantile section
			sampledMean,
			thisCDF,
			realCDF, // this column is equivalent to the cdf section
			sampledCDF,
			centroid.Weight,
			distanceToPrev,
			distanceToNext,
		))

		// this csv is equivalent to sizes.csv from dunning's tests
		sizes.Write(stringifySlice(
			distname,
			run,
			i,
			thisCDF,
			centroid.Weight,
		))
	}

	// now we compute errors for a fixed set of quantiles, as with errors.csv
	// in dunning's tests
	// we cover a wider range of quantiles just for the sake of completeness
	for i := 0; i <= 1000; i++ {
		quantile := float64(i) / 1000.0
		// find the real sample for the target quantile
		realQuantile := floatQuantile(allSamples, quantile)
		// find the estimated location of the target quantile
		estimatedQuantile := td.Quantile(quantile)
		// find the estimated cdf of the real sample
		estimatedCDF := td.CDF(realQuantile)
		errors.Write(stringifySlice(
			distname,
			run,
			quantile,
			estimatedCDF, // this column is equivalent to the cdf section
			realQuantile,
			estimatedQuantile, // this column is equivalent to the quantile section
		))
	}
}

func main() {
	// 10 * 100k is the default in dunning's original tests
	iterations := 10
	samplesPerIteration := 100000
	seed := time.Now().Unix()
	var err error
	switch len(os.Args) {
	case 4:
		seed, err = strconv.ParseInt(os.Args[3], 10, 64)
		if err != nil {
			panic(err)
		}
		fallthrough
	case 3:
		samplesPerIteration, err = strconv.Atoi(os.Args[2])
		if err != nil {
			panic(err)
		}
		fallthrough
	case 2:
		iterations, err = strconv.Atoi(os.Args[1])
		if err != nil {
			panic(err)
		}
	}
	fmt.Printf("Running for %d iterations of %d samples each with seed %d\n", iterations, samplesPerIteration, seed)

	deviationsFile, err := os.OpenFile("deviations.csv", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	deviations := csv.NewWriter(deviationsFile)
	defer finalizeCSV(deviationsFile, deviations)

	centroidErrorsFile, err := os.OpenFile("centroid-errors.csv", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	centroidErrors := csv.NewWriter(centroidErrorsFile)
	defer finalizeCSV(centroidErrorsFile, centroidErrors)

	errorsFile, err := os.OpenFile("errors.csv", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	errors := csv.NewWriter(errorsFile)
	defer finalizeCSV(errorsFile, errors)

	sizesFile, err := os.OpenFile("sizes.csv", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	sizes := csv.NewWriter(sizesFile)
	defer finalizeCSV(sizesFile, sizes)

	// write the column headers
	deviations.Write([]string{"dist", "run", "Q", "k", "x", "mean", "left", "right", "deviation"})
	centroidErrors.Write([]string{
		"dist",
		"run",
		"approx_mean",
		"real_mean_from_approx_cdf",
		"real_mean",
		"approx_cdf",
		"real_cdf_from_approx_mean",
		"real_cdf",
		"weight",
		"left",
		"right",
	})
	errors.Write([]string{"dist", "run", "Q", "approx_Q", "quantile", "approx_quantile"})
	sizes.Write([]string{"dist", "run", "i", "q", "actual"})

	rand.Seed(seed)
	for key, distribution := range map[string]func() float64{
		"uniform":     rand.Float64,
		"normal":      rand.NormFloat64,
		"exponential": rand.ExpFloat64,
	} {
		for i := 0; i < iterations; i++ {
			runOnce(distribution, 1000, samplesPerIteration, key, i, deviations, centroidErrors, errors, sizes)
		}
	}
}

func stringifySlice(s ...interface{}) []string {
	ret := make([]string, 0, len(s))
	for _, val := range s {
		switch v := val.(type) {
		case float64:
			ret = append(ret, strconv.FormatFloat(v, 'g', -1, 64))
		case int:
			ret = append(ret, strconv.Itoa(v))
		case string:
			ret = append(ret, v)
		default:
			panic(v)
		}
	}
	return ret
}

func floatQuantile(samples []float64, q float64) float64 {
	if len(samples) == 0 {
		return math.NaN()
	}

	index := int(float64(len(samples)) * q)
	if index < 0 {
		return samples[0]
	}
	if index >= len(samples) {
		return samples[len(samples)-1]
	}
	return samples[index]
}

func floatCDF(samples []float64, value float64) float64 {
	index := 0
	for ; index < len(samples) && samples[index] <= value; index++ {
	}
	return float64(index) / float64(len(samples))
}

func finalizeCSV(f *os.File, c *csv.Writer) {
	c.Flush()
	if err := c.Error(); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
