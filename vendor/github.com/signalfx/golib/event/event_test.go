package event

import (
	"testing"
	"time"

	"github.com/signalfx/com_signalfx_metrics_protobuf"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestEvent(t *testing.T) {
	ev := New("eventType", USERDEFINED, map[string]string{}, time.Now())
	assert.Contains(t, ev.String(), "eventType")
	assert.Contains(t, ev.String(), "1000000")
}

func TestToProtoEC(t *testing.T) {

	Convey("Invalid event categories should default to USERDEFINED", t, func() {
		resp := ToProtoEC(com_signalfx_metrics_protobuf.EventCategory(9999999))
		So(resp, ShouldEqual, USERDEFINED)
	})

	Convey("Event categories should match protobuf event categories", t, func() {
		var resp Category
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_USER_DEFINED)
		So(resp, ShouldEqual, USERDEFINED)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_ALERT)
		So(resp, ShouldEqual, ALERT)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_AUDIT)
		So(resp, ShouldEqual, AUDIT)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_JOB)
		So(resp, ShouldEqual, JOB)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_COLLECTD)
		So(resp, ShouldEqual, COLLECTD)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_SERVICE_DISCOVERY)
		So(resp, ShouldEqual, SERVICEDISCOVERY)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_EXCEPTION)
		So(resp, ShouldEqual, EXCEPTION)
		resp = ToProtoEC(com_signalfx_metrics_protobuf.EventCategory_AGENT)
		So(resp, ShouldEqual, AGENT)
	})
}
