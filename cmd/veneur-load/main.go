package main

import (
	"context"
	"flag"
	"log"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/trace"
	"sync/atomic"
	"time"

	"github.com/stripe/veneur/metricingester"
	"golang.org/x/time/rate"
)

func main() {
	qps := flag.Int("qps", 100, "Queries per second to make.")
	hostport := flag.String("hostport", "localhost:8200", "hostport to send requests to")
	parallelism := flag.Int("parallelism", runtime.NumCPU(), "number of workers to generate load")
	connections := flag.Int("connections", runtime.NumCPU(), "number of connections to share")
	flag.Parse()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// START LOAD GENERATOR

	vl := veneurLoadGenerator{
		f: metricingester.NewForwardingIngester(newRoundRobinConnector(*connections, *hostport)),
		m: metricingester.NewCounter("test", 10, []string{"abc:def"}, 1.0, "hostname"),
	}
	loader := newLoader(*qps, vl.run, *parallelism)
	loader.Start()
	start := time.Now()
	log.Printf("STARTING // requesting %s at %v qps", *hostport, *qps)

	// HANDLE SIGINT

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	cleanupDone := make(chan struct{})
	go func() {
		<-signalChan
		dur := time.Now().Sub(start)
		log.Fatalf(
			"FINISHED // made %d requests in %s // achieved %d qps",
			*loader.requestCount,
			dur,
			*loader.requestCount/uint64(math.Round(dur.Seconds())),
		)
		close(cleanupDone)
	}()
	<-cleanupDone
}

/*
 LOAD GENERATOR
*/

type loader struct {
	cb          func()
	limiter     *rate.Limiter
	parallelism int

	requestCount *uint64
	stopped      bool
}

func newLoader(qps int, cb func(), parallelism int) loader {
	return loader{
		cb:           cb,
		limiter:      rate.NewLimiter(rate.Limit(qps), qps),
		parallelism:  parallelism,
		requestCount: new(uint64),
	}
}

func (l *loader) Start() {
	for i := 0; i < l.parallelism; i++ {
		go l.runner()
	}
}

func (l *loader) runner() {
	for {
		ctx, task := trace.NewTask(context.Background(), "trace callback")
		if err := l.limiter.Wait(ctx); err != nil {
			log.Fatalf("error getting rate limit: %v", err)
		}
		trace.WithRegion(ctx, "loaderCallback", l.cb)
		atomic.AddUint64(l.requestCount, 1)
		task.End()
	}
}

/*
 * VENEUR CONNECTION GENERATOR
 */
// roundRobinConnector uses a round robin strategy to LB UDP file descriptors.
//
// this strategy scales better but causes contention on individual FDs.
type roundRobinConnector struct {
	conns []net.Conn
	count *uint64
}

func newRoundRobinConnector(nConns int, hostport string) roundRobinConnector {
	var conns []net.Conn
	for i := 0; i < nConns; i++ {
		conns = append(conns, dial(hostport))
	}
	return roundRobinConnector{conns: conns, count: new(uint64)}
}

func (s roundRobinConnector) Conn(ctx context.Context, hash string) (net.Conn, error) {
	atomic.AddUint64(s.count, 1)
	ind := atomic.LoadUint64(s.count) % uint64(len(s.conns))
	return s.conns[ind], nil
}

func (roundRobinConnector) Return(net.Conn, error) {
	return
}

// pooledConnector uses a buffered channel based pool.
//
// this strategy scales worse due to contention on single channel but performs
// better at low concurrency.
type pooledConnector struct {
	pool     chan net.Conn
	hostport string
}

func newPooledConnector(nConns int, hostport string) pooledConnector {
	ch := make(chan net.Conn, nConns)
	for i := 0; i < nConns; i++ {
		ch <- dial(hostport)
	}
	return pooledConnector{ch, hostport}
}

func (p pooledConnector) Conn(ctx context.Context, hash string) (net.Conn, error) {
	return <-p.pool, nil
}

func (p pooledConnector) Return(c net.Conn, err error) {
	p.pool <- c
}

/*
 * VENEUR REQUEST CODE
 */

type veneurLoadGenerator struct {
	f metricingester.ForwardingIngester
	m metricingester.Metric
}

func (v veneurLoadGenerator) run() {
	v.f.Ingest(context.Background(), v.m)
}

func dial(hostport string) net.Conn {
	conn, err := net.DialTimeout("udp", hostport, 1*time.Second)
	if err != nil {
		log.Fatalf("failed creating connection: %v", err)
	}
	return conn
}
