package diagnostics

import (
	"runtime"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/stripe/veneur/v14/scopedstatsd"
)

func setupMockMemstats(value uint64) runtime.MemStats {
	return runtime.MemStats{
		Sys:          value,
		Lookups:      value,
		Mallocs:      value,
		Frees:        value,
		TotalAlloc:   value,
		HeapAlloc:    value,
		HeapSys:      value,
		HeapIdle:     value,
		HeapInuse:    value,
		HeapReleased: value,
		HeapObjects:  value,
		StackInuse:   value,
		StackSys:     value,
		MSpanInuse:   value,
		MSpanSys:     value,
		MCacheInuse:  value,
		MCacheSys:    value,
		BuckHashSys:  value,
		GCSys:        value,
		OtherSys:     value,
		NextGC:       value,
	}
}

func TestUptimeMetricsInstrumentedWithRightValues(t *testing.T) {
	ctrl := gomock.NewController(t)

	defer ctrl.Finish()

	m := scopedstatsd.NewMockClient(ctrl)

	var interval time.Duration = time.Second
	var metricName string = "uptime_ms"
	var tags []string = []string{"git_sha:dirty"}
	var rate float64 = 1.0

	// Asserts that the first and only call to Count() is passed with correct parameters.
	m.
		EXPECT().
		Count(gomock.Eq(metricName), gomock.Eq(int64(1000)), gomock.Eq(tags), gomock.Eq(rate))

	CollectUptimeMetrics(m, interval, tags)
}

func TestMemoryMetricsInstrumentedWithRightValues(t *testing.T) {
	ctrl := gomock.NewController(t)

	defer ctrl.Finish()

	m := scopedstatsd.NewMockClient(ctrl)
	var tags []string = []string{"git_sha:dirty"}
	var rate float64 = 1
	var curMemstats = setupMockMemstats(100)
	var prevMemstats = setupMockMemstats(50)

	m.EXPECT().Gauge(gomock.Eq("sys_bytes"), gomock.Eq(float64(curMemstats.Sys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("pointer_lookups"), gomock.Eq(float64(curMemstats.Lookups)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Count(gomock.Eq("mallocs_total"), gomock.Eq(int64(curMemstats.Mallocs-prevMemstats.Mallocs)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Count(gomock.Eq("frees_total"), gomock.Eq(int64(curMemstats.Frees-prevMemstats.Frees)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("mallocs_count"), gomock.Eq(float64(curMemstats.Mallocs-curMemstats.Frees)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Count(gomock.Eq("heap_alloc_bytes_total"), gomock.Eq(int64(curMemstats.TotalAlloc-prevMemstats.TotalAlloc)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_alloc_bytes"), gomock.Eq(float64(curMemstats.HeapAlloc)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_sys_bytes"), gomock.Eq(float64(curMemstats.HeapSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_idle_bytes"), gomock.Eq(float64(curMemstats.HeapIdle)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_inuse_bytes"), gomock.Eq(float64(curMemstats.HeapInuse)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_released_bytes"), gomock.Eq(float64(curMemstats.HeapReleased)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("heap_objects_count"), gomock.Eq(float64(curMemstats.HeapObjects)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("stack_inuse_bytes"), gomock.Eq(float64(curMemstats.StackInuse)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("stack_sys_bytes"), gomock.Eq(float64(curMemstats.StackSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("mspan_inuse_bytes"), gomock.Eq(float64(curMemstats.MSpanInuse)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("mspan_sys_bytes"), gomock.Eq(float64(curMemstats.MSpanSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("mcache_inuse_bytes"), gomock.Eq(float64(curMemstats.MCacheInuse)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("mcache_sys_bytes"), gomock.Eq(float64(curMemstats.MCacheSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("buck_hash_sys_bytes"), gomock.Eq(float64(curMemstats.BuckHashSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("gc_sys_bytes"), gomock.Eq(float64(curMemstats.GCSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("other_sys_bytes"), gomock.Eq(float64(curMemstats.OtherSys)), gomock.Eq(tags), gomock.Eq(rate))
	m.EXPECT().Gauge(gomock.Eq("next_gc_bytes"), gomock.Eq(float64(curMemstats.NextGC)), gomock.Eq(tags), gomock.Eq(rate))

	CollectRuntimeMemStats(m, &curMemstats, &prevMemstats, tags)
}
