package splunk

// This entire file is ~copied from the HEC event definition found at
// <https://github.com/fuyufjh/splunk-hec-go/blob/3e5842a6937536b4f7221bc797f35500b1fa0ff7/event.go>.
// Licensed under Apache-2.0.

import (
	"fmt"
	"time"
)

type Event struct {
	Host       *string     `json:"host,omitempty"`
	Index      *string     `json:"index,omitempty"`
	Source     *string     `json:"source,omitempty"`
	SourceType *string     `json:"sourcetype,omitempty"`
	Time       *string     `json:"time,omitempty"`
	Event      interface{} `json:"event"`
}

func NewEvent(data interface{}) *Event {
	// Empty event is not allowed, but let HEC complain the error
	switch data := data.(type) {
	case *string:
		return &Event{Event: *data}
	case string:
		return &Event{Event: data}
	default:
		return &Event{Event: data}
	}
}

func (e *Event) SetHost(host string) {
	e.Host = &host
}

func (e *Event) SetIndex(index string) {
	e.Index = &index
}

func (e *Event) SetSourceType(sourcetype string) {
	e.SourceType = &sourcetype
}

func (e *Event) SetSource(source string) {
	e.Source = &source
}

func (e *Event) SetTime(time time.Time) {
	e.Time = String(epochTime(&time))
}

func epochTime(t *time.Time) string {
	millis := t.UnixNano() / 1000000
	return fmt.Sprintf("%d.%03d", millis/1000, millis%1000)
}

func String(str string) *string {
	return &str
}
