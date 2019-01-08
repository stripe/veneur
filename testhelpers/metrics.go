package testhelpers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"testing"
)

func RandomString(tb testing.TB, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	bts := make([]byte, length)
	for i := range bts {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			tb.Fatalf("error generating random data: %s", err)
		}

		bts[i] = charset[n.Int64()]
	}
	return string(bts)
}

func GenerateRandomStatsdMetricPackets(tb testing.TB, length int) [][]byte {
	const MetricFormat = "%s:%d|%s|#%s"

	packets := make([][]byte, length)

	for i, _ := range packets {
		p := make([]byte, 30)
		_, err := rand.Read(p)
		if err != nil {
			tb.Fatalf("Error generating data: %s", err)
		}

		metricName := RandomString(tb, 10)
		metricValue := rune(RandomString(tb, 1)[0])
		metricType := "c"
		if metricValue%2 == 0 {
			// make half of them timers
			metricType = "ms"
		}
		metricTags := []string{
			fmt.Sprintf("%s:%s", RandomString(tb, 10), RandomString(tb, 8)),
			fmt.Sprintf("%s:%s", RandomString(tb, 7), RandomString(tb, 8)),
			fmt.Sprintf("%s:%s", RandomString(tb, 9), RandomString(tb, 18)),
			fmt.Sprintf("%s:%s", RandomString(tb, 1), RandomString(tb, 28)),
			fmt.Sprintf("%s:%s", RandomString(tb, 4), RandomString(tb, 9)),
			fmt.Sprintf("%s:%s", RandomString(tb, 20), RandomString(tb, 19)),
			fmt.Sprintf("%s:%s", RandomString(tb, 6), RandomString(tb, 20)),
		}

		if metricValue%11 == 0 {
			metricTags = append(metricTags, "veneurglobalonly")
		}

		if metricValue%13 == 0 {
			metricTags = append(metricTags, "veneurlocalonly")
		}

		metricTagsJoined := strings.Join(metricTags, ",")

		packets[i] = []byte(fmt.Sprintf(MetricFormat, metricName, metricValue, metricType, metricTagsJoined))

	}
	return packets
}
