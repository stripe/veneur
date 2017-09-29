package log_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/signalfx/golib/log"
	"gopkg.in/logfmt.v0"
)

func TestLogfmtLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.NewLogfmtLogger(buf, &panicLogger{})

	logger.Log("hello", "world")
	if want, have := "hello=world\n", buf.String(); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}

	buf.Reset()
	logger.Log("a", 1, "err", errors.New("error"))
	if want, have := "a=1 err=error\n", buf.String(); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}

	buf.Reset()
	logger.Log("std_map", map[int]int{1: 2}, "my_map", mymap{0: 0})
	if want, have := "std_map=\""+logfmt.ErrUnsupportedValueType.Error()+"\" my_map=special_behavior\n", buf.String(); want != have {
		t.Errorf("want %#v, have %#v", want, have)
	}
}

func BenchmarkLogfmtLoggerSimple(b *testing.B) {
	benchmarkRunner(b, log.NewLogfmtLogger(ioutil.Discard, log.Discard), baseMessage)
}

func BenchmarkLogfmtLoggerContextual(b *testing.B) {
	benchmarkRunner(b, log.NewLogfmtLogger(ioutil.Discard, log.Discard), withMessage)
}

func TestLogfmtLoggerConcurrency(t *testing.T) {
	testConcurrency(t, log.NewLogfmtLogger(ioutil.Discard, log.Discard))
}

type mymap map[int]int

func (m mymap) String() string { return "special_behavior" }
