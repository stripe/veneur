package veneur

import (
	"sync/atomic"
	"time"

	rtdebug "runtime/debug"

	"github.com/sirupsen/logrus"
)

type watchdogTickerMetadata struct {
	FlushGroup     string
	Ticker         *time.Ticker
	StuckIntervals int
	Interval       time.Duration
}

// FlushWatchdog periodically checks that at most
// `flush_watchdog_missed_flushes` were skipped in a Server. If more
// than that number was skipped, it panics (assuming that flushing is
// stuck) with a full level of detail on that panic's backtraces.
//
// It never terminates, so is ideally run from a goroutine in a
// program's main function.
func (s *Server) FlushWatchdog() {
	tickers := make([]watchdogTickerMetadata, 0, len(s.computationRoutingConfig))
	for _, conf := range s.computationRoutingConfig {
		if conf.WorkerWatchdogIntervals == 0 {
			// No watchdog needed:
			return
		}
		tickers = append(tickers, watchdogTickerMetadata{
			conf.FlushGroup,
			time.NewTicker(conf.WorkerInterval),
			conf.WorkerWatchdogIntervals,
			conf.WorkerInterval,
		})
		atomic.StoreInt64(s.lastFlushTsByFlushGroup[conf.FlushGroup], time.Now().UnixNano())
	}

	// Promote panics outside of the goroutines this function spawns, such that we can assert
	// the watchdog panics in automated tests
	recoveredPanicCh := make(chan interface{})

	for _, ticker := range tickers {
		go func(metadata watchdogTickerMetadata) {
			defer func() {
				recoveredPanicCh <- recover()
			}()

			for {
				select {
				case <-s.shutdown:
					metadata.Ticker.Stop()
					return
				case <-metadata.Ticker.C:
					last := time.Unix(0, atomic.LoadInt64(s.lastFlushTsByFlushGroup[metadata.FlushGroup]))
					since := time.Since(last)

					// If no flush was kicked off in the last N
					// times, we're stuck - panic because that's a
					// bug.
					if since > time.Duration(metadata.StuckIntervals)*metadata.Interval {
						rtdebug.SetTraceback("all")
						log.WithFields(logrus.Fields{
							"last_flush":       last,
							"missed_intervals": s.stuckIntervals,
							"time_since":       since,
						}).
							Panic("Flushing seems to be stuck. Terminating.")
					}
				}
			}
		}(ticker)
	}
	recoveredPanic := <-recoveredPanicCh
	ConsumePanic(s.TraceClient, s.Hostname, recoveredPanic)
}
