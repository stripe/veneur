package metricingester

import "testing"

func BenchmarkWorkerIngest(b *testing.B) {
	w := newAggWorker()
	w.Start()
	// we don't want slice allocation to be part of the benchmark since this is normally done
	// outside of creating a metric.
	tags := []string{"c:d", "f:g", "a:b"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Ingest(NewCounter("mycounter", 100, tags, 1.0, "myhost"))
	}
}

func BenchmarkMetric_Hash(b *testing.B) {
	m := NewCounter("test", 1, []string{
		"host:abcdefghjiklmn",
		"merchant:abcdefghjijkkl",
		"moretags:heyheyheyehey",
		"host:abcdefghjiklmn",
		"merchant:abcdefghjijkkl",
		"moretags:heyheyheyehey",
		"host:abcdefghjiklmn",
		"merchant:abcdefghjijkkl",
		"moretags:heyheyheyehey",
	}, 1.0, "myhost")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Hash()
	}
}
