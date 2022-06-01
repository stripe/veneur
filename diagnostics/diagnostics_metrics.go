package diagnostics

import (
	"runtime"
	"time"

	"github.com/stripe/veneur/v14"
)

var gitSha string = "git_sha: " + veneur.VERSION

func CollectDiagnosticsMetrics(s *veneur.Server) {
	var memstatsCurrent runtime.MemStats
	var memstatsPrev runtime.MemStats

	for range time.Tick(s.Interval) {
		CollectUptimeMetrics(s)
		CollectRuntimeMemStats(s, &memstatsCurrent, &memstatsPrev)
		memstatsPrev = memstatsCurrent
	}
}

func CollectUptimeMetrics(s *veneur.Server) {
	s.Statsd.Count("uptime_ms", s.Interval.Milliseconds(), []string{gitSha}, 1)
}

// This function take inspiration from the prometheus golang code base: https://github.com/prometheus/client_golang/blob/24172847e35ba46025c49d90b8846b59eb5d9ead/prometheus/go_collector.go
func CollectRuntimeMemStats(s *veneur.Server, memstatsCurrentPtr *runtime.MemStats, memstatsPrevPtr *runtime.MemStats) {

	runtime.ReadMemStats(memstatsCurrentPtr)
	var memstatsCurrent = *memstatsCurrentPtr
	var memstatsPrev = *memstatsPrevPtr

	// Collect number of bytes obtained from system.
	s.Statsd.Gauge("current_sys_bytes", float64(memstatsCurrent.Sys), []string{gitSha}, 1)

	// Collect number of pointer lookups.
	s.Statsd.Gauge("current_pointer_lookups", float64(memstatsCurrent.Lookups), []string{gitSha}, 1)

	// Collect increased heap objects allocated compared to last flush.
	s.Statsd.Count("increased_mallocs", int64(memstatsCurrent.Mallocs-memstatsPrev.Mallocs), []string{gitSha}, 1)

	// Collect increased heap objects freed compared to last flush.
	s.Statsd.Count("increased_frees", int64(memstatsCurrent.Frees-memstatsPrev.Frees), []string{gitSha}, 1)

	// Collect number of mallocs.
	s.Statsd.Gauge("current_mallocs_count", float64(memstatsCurrent.Mallocs-memstatsCurrent.Frees), []string{gitSha}, 1)

	// Collect number of bytes newly allocated for heap objects compared to last flush.
	s.Statsd.Count("increased_heap_alloc_bytes", int64(memstatsCurrent.TotalAlloc-memstatsPrev.TotalAlloc), []string{gitSha}, 1)

	// Collect number of heap bytes allocated and still in use.
	s.Statsd.Gauge("current_heap_alloc_bytes", float64(memstatsCurrent.HeapAlloc), []string{gitSha}, 1)

	// Collect number of heap bytes obtained from system.
	s.Statsd.Gauge("current_heap_sys_bytes", float64(memstatsCurrent.HeapSys), []string{gitSha}, 1)

	// Collect number of heap bytes waiting to be used.
	s.Statsd.Gauge("current_heap_idle_bytes", float64(memstatsCurrent.HeapIdle), []string{gitSha}, 1)

	// Collect number of heap bytes that are in use.
	s.Statsd.Gauge("current_heap_inuse_bytes", float64(memstatsCurrent.HeapInuse), []string{gitSha}, 1)

	// Collect number of heap bytes released to OS.
	s.Statsd.Gauge("current_heap_released_bytes", float64(memstatsCurrent.HeapReleased), []string{gitSha}, 1)

	// Collect number of allocated objects.
	s.Statsd.Gauge("current_heap_objects_count", float64(memstatsCurrent.HeapObjects), []string{gitSha}, 1)

	// Collect number of bytes in use by the stack allocator.
	s.Statsd.Gauge("current_stack_inuse_bytes", float64(memstatsCurrent.StackInuse), []string{gitSha}, 1)

	// Collect number of bytes obtained from system for stack allocator.
	s.Statsd.Gauge("current_stack_sys_bytes", float64(memstatsCurrent.StackSys), []string{gitSha}, 1)

	// Collect number of bytes in use by mspan structures.
	s.Statsd.Gauge("current_mspan_inuse_bytes", float64(memstatsCurrent.MSpanInuse), []string{gitSha}, 1)

	// Collect number of bytes used for mspan structures obtained from system.
	s.Statsd.Gauge("current_mspan_sys_bytes", float64(memstatsCurrent.MSpanSys), []string{gitSha}, 1)

	// Collect number of bytes in use by mcache structures.
	s.Statsd.Gauge("current_mcache_inuse_bytes", float64(memstatsCurrent.MCacheInuse), []string{gitSha}, 1)

	// Collect number of bytes used for mcache structures obtained from system.
	s.Statsd.Gauge("current_mcache_sys_bytes", float64(memstatsCurrent.MCacheSys), []string{gitSha}, 1)

	// Collect number of bytes used by the profiling bucket hash table.
	s.Statsd.Gauge("current_buck_hash_sys_bytes", float64(memstatsCurrent.BuckHashSys), []string{gitSha}, 1)

	// Collect number of bytes used for garbage collection system metadata.
	s.Statsd.Gauge("current_gc_sys_bytes", float64(memstatsCurrent.GCSys), []string{gitSha}, 1)

	// Collect number of bytes used for other system allocations.
	s.Statsd.Gauge("current_other_sys_bytes", float64(memstatsCurrent.OtherSys), []string{gitSha}, 1)

	// Collect number of heap bytes when next garbage collection will take pace.
	s.Statsd.Gauge("next_gc_bytes", float64(memstatsCurrent.NextGC), []string{gitSha}, 1)
}
