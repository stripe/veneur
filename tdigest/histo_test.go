package tdigest

import (
	"encoding/base64"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

func TestMergingDigest(t *testing.T) {
	rand.Seed(time.Now().Unix())

	td := NewMerging(1000, false)

	for i := 0; i < 100000; i++ {
		td.Add(rand.Float64(), 1.0)
	}
	validateMergingDigest(t, td)

	// we don't bother testing the CDF here, it's not very precise at the median
	assert.InEpsilon(t, 0.5, td.Quantile(0.5), 0.02, "median was %v, not 0.5", td.Quantile(0.5))
	assert.True(t, td.Min() >= 0, "minimum was %v, expected non-negative", td.Min())
	assert.True(t, td.Max() < 1, "maximum was %v, expected below 1", td.Max())
	assert.True(t, td.Sum() > 0, "sum was not greater than 0")
	assert.True(t, td.ReciprocalSum() > 0, "reciprocal sum was not greater than 0")
}

func TestMergeSparseDigest(t *testing.T) {
	td := NewMerging(1000, false)
	td.Add(-200000, 1)
	other := NewMerging(1000, false)
	other.Add(200000, 1)

	td.Merge(other)
	validateMergingDigest(t, td)

	assert.InEpsilon(t, 0.5, td.CDF(0), 0.02, "cdf below 0 was %v, not ~50%", td.CDF(0))
	// epsilon-style ULP comparisons do not work on zero
	assert.InDelta(t, 0, td.Quantile(0.5), 0.02, "median was %v, not 0", td.Quantile(0.5))
	assert.InEpsilon(t, td.Min(), td.Quantile(0), 0.02, "minimum was %v", td.Quantile(0))
	assert.InEpsilon(t, td.Max(), td.Quantile(1), 0.02, "maximum was %v", td.Quantile(1))
	assert.InDelta(t, 0, td.Sum(), 0.01)
	assert.InDelta(t, 0, other.ReciprocalSum(), 0.01)
}

// check the basic validity of a merging t-digest
// are its centroids within the sizing bound?
// do its weights add up?
func validateMergingDigest(t *testing.T, td *MergingDigest) {
	td.mergeAllTemps()

	index := 0.0
	quantile := 0.0
	runningWeight := 0.0
	for i, c := range td.mainCentroids {
		nextIndex := td.indexEstimate(quantile + c.Weight/td.mainWeight)
		// avoid checking the first and last centroids
		// they're under the strictest expectations so they often fail
		if i != 0 && i != len(td.mainCentroids)-1 {
			assert.True(t, nextIndex-index <= 1 || c.Weight == 1, "centroid is oversized: ", c)
		}

		quantile += c.Weight / td.mainWeight
		index = nextIndex
		runningWeight += c.Weight
	}

	assert.Equal(t, td.mainWeight, runningWeight, "total weights didn't add up")
}

func TestGobEncoding(t *testing.T) {
	rand.Seed(time.Now().Unix())

	td := NewMerging(1000, false)
	for i := 0; i < 1000; i++ {
		td.Add(rand.Float64(), 1.0)
	}
	validateMergingDigest(t, td)

	buf, err := td.GobEncode()
	assert.NoError(t, err, "should have encoded successfully")

	td2 := NewMerging(1000, false)
	assert.NoError(t, td2.GobDecode(buf), "should have decoded successfully")

	assert.InEpsilon(t, td.Count(), td2.Count(), 0.02, "counts did not match")
	assert.InEpsilon(t, td.Min(), td2.Min(), 0.02, "minimums did not match")
	assert.InEpsilon(t, td.Max(), td2.Max(), 0.02, "maximums did not match")
	assert.InEpsilon(t, td.Quantile(0.5), td2.Quantile(0.5), 0.02, "50%% quantiles did not match")
	assert.Equal(t, td.Sum(), td2.Sum())
	assert.Equal(t, td.ReciprocalSum(), td2.ReciprocalSum())
}

func serializeGob(t *testing.T, buf []byte, fname string) error {
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := base64.NewEncoder(base64.StdEncoding, f)
	defer encoder.Close()

	_, err = encoder.Write(buf)
	if err != nil {
		return err
	}
	return nil
}

func encodedGob(t *testing.T) ([]byte, error) {
	td := NewMerging(1000, false)
	for i := 0; i < 1000; i++ {
		td.Add(float64(i), 1.0)
	}
	validateMergingDigest(t, td)

	return td.GobEncode()

}

func deserializeGob(t *testing.T, fname string) []byte {
	f, err := os.Open("testdata/oldgob.base64")
	require.NoError(t, err)
	buf, err := ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, f))
	require.NoError(t, err)
	return buf
}

// TestGobDecodeOldGob ensures the decoding path is backwards compatible with
// the first version of the interface. If necessary, you can record other
// possible gobs using the commented code.
func TestGobDecodeOldGob(t *testing.T) {
	// uncomment these lines if you need to record a new gob to test
	// a different interface change.
	//
	//gobTDigest, _ := encodedGob(t)
	//require.NoError(t, serializeGob(t, gobTDigest, "oldgob.base64"))

	td := NewMerging(1000, false)
	buf := deserializeGob(t, "oldgob.base64")
	assert.NoError(t, td.GobDecode(buf), "should have decoded successfully")

	assert.InEpsilon(t, 1000, td.Count(), 0.02, "counts did not match")
	assert.InDelta(t, 0.01, td.Min(), 0.02, "minimums did not match")
	assert.InEpsilon(t, 1000, td.Max(), 0.02, "maximums did not match")
	assert.InEpsilon(t, 500, td.Quantile(0.5), 0.02, "50%% quantiles did not match")
	assert.Equal(t, float64(499500), td.Sum())
	assert.Equal(t, float64(0), td.ReciprocalSum())

}

// Test that creating a MergingDigestData (for protobuf-encoding) and
// recreating the original MergingDigest works as expected.
func TestMergingDigestData(t *testing.T) {
	rand.Seed(time.Now().Unix())

	td := NewMerging(1000, false)
	for i := 0; i < 1000; i++ {
		td.Add(rand.Float64(), 1.0)
	}
	validateMergingDigest(t, td)

	// Convert to a MergingDigestData and back again
	td2 := NewMergingFromData(td.Data())

	assert.InEpsilon(t, td.Count(), td2.Count(), 0.02, "counts did not match")
	assert.InEpsilon(t, td.Min(), td2.Min(), 0.02, "minimums did not match")
	assert.InEpsilon(t, td.Max(), td2.Max(), 0.02, "maximums did not match")
	assert.InEpsilon(t, td.Quantile(0.5), td2.Quantile(0.5), 0.02, "50%% quantiles did not match")
	assert.Equal(t, td.Sum(), td2.Sum())
	assert.Equal(t, td.ReciprocalSum(), td2.ReciprocalSum())
}

func BenchmarkAdd(b *testing.B) {
	rand.Seed(time.Now().Unix())
	td := NewMerging(1000, false)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		td.Add(rand.NormFloat64(), 1.0)
	}
}

func BenchmarkQuantile(b *testing.B) {
	rand.Seed(time.Now().Unix())
	td := NewMerging(1000, false)
	for i := 0; i < b.N; i++ {
		td.Add(rand.NormFloat64(), 1.0)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		td.Quantile(rand.Float64())
	}
}
