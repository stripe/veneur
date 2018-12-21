package metricingester_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/metricingester"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

type connecter struct {
	target string
}

func (c connecter) Conn(ctx context.Context, hash string) (net.Conn, error) {
	return net.Dial("udp", c.target)
}

func (connecter) Error(net.Conn, error) {
}

var cases = []struct {
	in  metricingester.Metric
	out *ssf.SSFSpan
	msg string
}{
	{
		counter("a", 1),
		&ssf.SSFSpan{Metrics: []*ssf.SSFSample{ssf.Count("a", 1, nil)}},
		"basic counter failed",
	},
}

func TestForwardingIngester_Ingest(t *testing.T) {
	for _, c := range cases {
		t.Run(c.msg, test(c.in, c.out))
	}
}

func test(in metricingester.Metric, out *ssf.SSFSpan) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// SET UP SERVER
		l, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
		laddr := l.LocalAddr()
		rc := make(chan *ssf.SSFSpan)

		go func() {
			ssf, err := protocol.ReadSSF(l)
			require.NoError(t, err)
			rc <- ssf
		}()

		// SET UP INGESTER
		fwd := metricingester.NewForwardingIngester(connecter{laddr.String()})

		// EXECUTE TEST
		require.NoError(t, fwd.Ingest(context.Background(), in))

		// COLLECT AND ASSERT RESULTS
		result := <-rc
		assert.Equal(t, result, out)
	}
}
