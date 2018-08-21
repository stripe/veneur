package hec

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
	switch data.(type) {
	case *string:
		return &Event{Event: *data.(*string)}
	case string:
		return &Event{Event: data.(string)}
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

func (e *Event) empty() bool {
	switch e.Event.(type) {
	case *string:
		return e.Event.(*string) == nil || *e.Event.(*string) == ""
	case string:
		return e.Event.(string) == ""
	default:
		return e.Event == nil
	}
}

func epochTime(t *time.Time) string {
	millis := t.UnixNano() / 1000000
	return fmt.Sprintf("%d.%03d", millis/1000, millis%1000)
}

func String(str string) *string {
	return &str
}
