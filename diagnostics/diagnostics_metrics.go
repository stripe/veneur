package diagnostics

import (
	"runtime"
	"time"

	"github.com/stripe/veneur/v14"
)

func CollectUptimeMetrics(s *veneur.Server) {
	for range time.Tick(s.Interval) {
		s.Statsd.Count("uptime_ms", s.Interval.Milliseconds(), []string{"commit_version:" + veneur.VERSION}, 1)
	}
}

// This function take inspiration from the prometheus golang code base: https://github.com/prometheus/client_golang/blob/24172847e35ba46025c49d90b8846b59eb5d9ead/prometheus/go_collector.go
func CollectRuntimeMemStats(s *veneur.Server) {
	var m runtime.MemStats
	for range time.Tick(s.Interval) {
		runtime.ReadMemStats(&m)

		// Collect number of bytes allocated and still in use.
		s.Statsd.Gauge("alloc_bytes", float64(m.Alloc), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect total number of bytes allocated, even if freed.
		s.Statsd.Gauge("alloc_bytes_total", float64(m.TotalAlloc), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes obtained from system.
		s.Statsd.Gauge("sys_bytes", float64(m.Sys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect total number of pointer lookups.
		s.Statsd.Gauge("lookups_total", float64(m.Lookups), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect total number of mallocs.
		s.Statsd.Gauge("mallocs_total", float64(m.Mallocs), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect total number of frees.
		s.Statsd.Gauge("frees_total", float64(m.Frees), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes allocated and still in use.
		s.Statsd.Gauge("heap_alloc_bytes", float64(m.HeapAlloc), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes obtained from system.
		s.Statsd.Gauge("heap_sys_bytes", float64(m.HeapSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes waiting to be used.
		s.Statsd.Gauge("heap_idle_bytes", float64(m.HeapIdle), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes that are in use.
		s.Statsd.Gauge("heap_inuse_bytes", float64(m.HeapInuse), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes released to OS.
		s.Statsd.Gauge("heap_released_bytes", float64(m.HeapReleased), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of allocated objects.
		s.Statsd.Gauge("heap_objects", float64(m.HeapObjects), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes in use by the stack allocator.
		s.Statsd.Gauge("stack_inuse_bytes", float64(m.StackInuse), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes obtained from system for stack allocator.
		s.Statsd.Gauge("stack_sys_bytes", float64(m.StackSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes in use by mspan structures.
		s.Statsd.Gauge("mspan_inuse_bytes", float64(m.MSpanInuse), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes used for mspan structures obtained from system.
		s.Statsd.Gauge("mspan_sys_bytes", float64(m.MSpanSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes in use by mcache structures.
		s.Statsd.Gauge("mcache_inuse_bytes", float64(m.MCacheInuse), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes used for mcache structures obtained from system.
		s.Statsd.Gauge("mcache_sys_bytes", float64(m.MCacheSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes used by the profiling bucket hash table.
		s.Statsd.Gauge("buck_hash_sys_bytes", float64(m.BuckHashSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes used for garbage collection system metadata.
		s.Statsd.Gauge("gc_sys_bytes", float64(m.GCSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of bytes used for other system allocations.
		s.Statsd.Gauge("other_sys_bytes", float64(m.OtherSys), []string{"commit_version:" + veneur.VERSION}, 1)

		// Collect number of heap bytes when next garbage collection will take place.
		s.Statsd.Gauge("next_gc_bytes", float64(m.NextGC), []string{"commit_version:" + veneur.VERSION}, 1)

	}
}
