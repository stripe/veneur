package diagnostics

import (
	"context"
	"runtime"
	"time"

	"github.com/stripe/veneur/v14/scopedstatsd"
)

func CollectDiagnosticsMetrics(
	ctx context.Context, statsd scopedstatsd.Client, interval time.Duration,
	tags []string,
) {
	var memstatsCurrent runtime.MemStats
	var memstatsPrev runtime.MemStats

	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			CollectUptimeMetrics(statsd, interval, tags)
			runtime.ReadMemStats(&memstatsCurrent)
			CollectRuntimeMemStats(statsd, &memstatsCurrent, &memstatsPrev, tags)
			memstatsPrev = memstatsCurrent
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}

func CollectUptimeMetrics(statsd scopedstatsd.Client, interval time.Duration, tags []string) {
	statsd.Count("uptime_ms", interval.Milliseconds(), tags, 1)
}

// This function take inspiration from the prometheus golang code base: https://github.com/prometheus/client_golang/blob/24172847e35ba46025c49d90b8846b59eb5d9ead/prometheus/go_collector.go
func CollectRuntimeMemStats(statsd scopedstatsd.Client, memstatsCurrent *runtime.MemStats, memstatsPrev *runtime.MemStats, tags []string) {
	// Collect number of bytes obtained from system.
	statsd.Gauge("mem.sys_bytes", float64(memstatsCurrent.Sys), tags, 1)

	// Collect number of pointer lookups.
	statsd.Gauge("mem.pointer_lookups", float64(memstatsCurrent.Lookups), tags, 1)

	// Collect increased heap objects allocated compared to last flush.
	statsd.Count("mem.mallocs_total", int64(memstatsCurrent.Mallocs-memstatsPrev.Mallocs), tags, 1)

	// Collect increased heap objects freed compared to last flush.
	statsd.Count("mem.frees_total", int64(memstatsCurrent.Frees-memstatsPrev.Frees), tags, 1)

	// Collect number of mallocs.
	statsd.Gauge("mem.mallocs_count", float64(memstatsCurrent.Mallocs-memstatsCurrent.Frees), tags, 1)

	// Collect number of bytes newly allocated for heap objects compared to last flush.
	statsd.Count("mem.heap_alloc_bytes_total", int64(memstatsCurrent.TotalAlloc-memstatsPrev.TotalAlloc), tags, 1)

	// Collect number of heap bytes allocated and still in use.
	statsd.Gauge("mem.heap_alloc_bytes", float64(memstatsCurrent.HeapAlloc), tags, 1)

	// Collect number of heap bytes obtained from system.
	statsd.Gauge("mem.heap_sys_bytes", float64(memstatsCurrent.HeapSys), tags, 1)

	// Collect number of heap bytes waiting to be used.
	statsd.Gauge("mem.heap_idle_bytes", float64(memstatsCurrent.HeapIdle), tags, 1)

	// Collect number of heap bytes that are in use.
	statsd.Gauge("mem.heap_inuse_bytes", float64(memstatsCurrent.HeapInuse), tags, 1)

	// Collect number of heap bytes released to OS.
	statsd.Gauge("mem.heap_released_bytes", float64(memstatsCurrent.HeapReleased), tags, 1)

	// Collect number of allocated objects.
	statsd.Gauge("mem.heap_objects_count", float64(memstatsCurrent.HeapObjects), tags, 1)

	// Collect number of bytes in use by the stack allocator.
	statsd.Gauge("mem.stack_inuse_bytes", float64(memstatsCurrent.StackInuse), tags, 1)

	// Collect number of bytes obtained from system for stack allocator.
	statsd.Gauge("mem.stack_sys_bytes", float64(memstatsCurrent.StackSys), tags, 1)

	// Collect number of bytes in use by mspan structures.
	statsd.Gauge("mem.mspan_inuse_bytes", float64(memstatsCurrent.MSpanInuse), tags, 1)

	// Collect number of bytes used for mspan structures obtained from system.
	statsd.Gauge("mem.mspan_sys_bytes", float64(memstatsCurrent.MSpanSys), tags, 1)

	// Collect number of bytes in use by mcache structures.
	statsd.Gauge("mem.mcache_inuse_bytes", float64(memstatsCurrent.MCacheInuse), tags, 1)

	// Collect number of bytes used for mcache structures obtained from system.
	statsd.Gauge("mem.mcache_sys_bytes", float64(memstatsCurrent.MCacheSys), tags, 1)

	// Collect number of bytes used by the profiling bucket hash table.
	statsd.Gauge("mem.buck_hash_sys_bytes", float64(memstatsCurrent.BuckHashSys), tags, 1)

	// Collect number of bytes used for garbage collection system metadata.
	statsd.Gauge("mem.gc_sys_bytes", float64(memstatsCurrent.GCSys), tags, 1)

	// Collect number of bytes used for other system allocations.
	statsd.Gauge("mem.other_sys_bytes", float64(memstatsCurrent.OtherSys), tags, 1)

	// Collect number of heap bytes when next garbage collection will take pace.
	statsd.Gauge("mem.next_gc_bytes", float64(memstatsCurrent.NextGC), tags, 1)
}
