// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: samplers/metricpb/metric.proto

package metricpb

import (
	encoding_binary "encoding/binary"
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	tdigest "github.com/stripe/veneur/tdigest"
	io "io"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

// Scope describes at which level the metric will be emitted.
type Scope int32

const (
	Scope_Mixed  Scope = 0
	Scope_Local  Scope = 1
	Scope_Global Scope = 2
)

var Scope_name = map[int32]string{
	0: "Mixed",
	1: "Local",
	2: "Global",
}

var Scope_value = map[string]int32{
	"Mixed":  0,
	"Local":  1,
	"Global": 2,
}

func (x Scope) String() string {
	return proto.EnumName(Scope_name, int32(x))
}

func (Scope) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{0}
}

// Type can be any of the valid metric types recognized by Veneur.
type Type int32

const (
	Type_Counter   Type = 0
	Type_Gauge     Type = 1
	Type_Histogram Type = 2
	Type_Set       Type = 3
	Type_Timer     Type = 4
)

var Type_name = map[int32]string{
	0: "Counter",
	1: "Gauge",
	2: "Histogram",
	3: "Set",
	4: "Timer",
}

var Type_value = map[string]int32{
	"Counter":   0,
	"Gauge":     1,
	"Histogram": 2,
	"Set":       3,
	"Timer":     4,
}

func (x Type) String() string {
	return proto.EnumName(Type_name, int32(x))
}

func (Type) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{1}
}

// Metric is a common container for any metric type. Common fields such as
// Name, Tags, and Type are all present for all types, while the value can
// vary.
type Metric struct {
	Name string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Tags []string `protobuf:"bytes,2,rep,name=tags,proto3" json:"tags,omitempty"`
	Type Type     `protobuf:"varint,3,opt,name=type,proto3,enum=metricpb.Type" json:"type,omitempty"`
	// Types that are valid to be assigned to Value:
	//	*Metric_Counter
	//	*Metric_Gauge
	//	*Metric_Histogram
	//	*Metric_Set
	Value isMetric_Value `protobuf_oneof:"value"`
	Scope Scope          `protobuf:"varint,9,opt,name=scope,proto3,enum=metricpb.Scope" json:"scope,omitempty"`
}

func (m *Metric) Reset()         { *m = Metric{} }
func (m *Metric) String() string { return proto.CompactTextString(m) }
func (*Metric) ProtoMessage()    {}
func (*Metric) Descriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{0}
}
func (m *Metric) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Metric) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Metric.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Metric) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Metric.Merge(m, src)
}
func (m *Metric) XXX_Size() int {
	return m.Size()
}
func (m *Metric) XXX_DiscardUnknown() {
	xxx_messageInfo_Metric.DiscardUnknown(m)
}

var xxx_messageInfo_Metric proto.InternalMessageInfo

type isMetric_Value interface {
	isMetric_Value()
	MarshalTo([]byte) (int, error)
	Size() int
}

type Metric_Counter struct {
	Counter *CounterValue `protobuf:"bytes,5,opt,name=counter,proto3,oneof"`
}
type Metric_Gauge struct {
	Gauge *GaugeValue `protobuf:"bytes,6,opt,name=gauge,proto3,oneof"`
}
type Metric_Histogram struct {
	Histogram *HistogramValue `protobuf:"bytes,7,opt,name=histogram,proto3,oneof"`
}
type Metric_Set struct {
	Set *SetValue `protobuf:"bytes,8,opt,name=set,proto3,oneof"`
}

func (*Metric_Counter) isMetric_Value()   {}
func (*Metric_Gauge) isMetric_Value()     {}
func (*Metric_Histogram) isMetric_Value() {}
func (*Metric_Set) isMetric_Value()       {}

func (m *Metric) GetValue() isMetric_Value {
	if m != nil {
		return m.Value
	}
	return nil
}

func (m *Metric) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Metric) GetTags() []string {
	if m != nil {
		return m.Tags
	}
	return nil
}

func (m *Metric) GetType() Type {
	if m != nil {
		return m.Type
	}
	return Type_Counter
}

func (m *Metric) GetCounter() *CounterValue {
	if x, ok := m.GetValue().(*Metric_Counter); ok {
		return x.Counter
	}
	return nil
}

func (m *Metric) GetGauge() *GaugeValue {
	if x, ok := m.GetValue().(*Metric_Gauge); ok {
		return x.Gauge
	}
	return nil
}

func (m *Metric) GetHistogram() *HistogramValue {
	if x, ok := m.GetValue().(*Metric_Histogram); ok {
		return x.Histogram
	}
	return nil
}

func (m *Metric) GetSet() *SetValue {
	if x, ok := m.GetValue().(*Metric_Set); ok {
		return x.Set
	}
	return nil
}

func (m *Metric) GetScope() Scope {
	if m != nil {
		return m.Scope
	}
	return Scope_Mixed
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*Metric) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _Metric_OneofMarshaler, _Metric_OneofUnmarshaler, _Metric_OneofSizer, []interface{}{
		(*Metric_Counter)(nil),
		(*Metric_Gauge)(nil),
		(*Metric_Histogram)(nil),
		(*Metric_Set)(nil),
	}
}

func _Metric_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*Metric)
	// value
	switch x := m.Value.(type) {
	case *Metric_Counter:
		_ = b.EncodeVarint(5<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Counter); err != nil {
			return err
		}
	case *Metric_Gauge:
		_ = b.EncodeVarint(6<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Gauge); err != nil {
			return err
		}
	case *Metric_Histogram:
		_ = b.EncodeVarint(7<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Histogram); err != nil {
			return err
		}
	case *Metric_Set:
		_ = b.EncodeVarint(8<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.Set); err != nil {
			return err
		}
	case nil:
	default:
		return fmt.Errorf("Metric.Value has unexpected type %T", x)
	}
	return nil
}

func _Metric_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*Metric)
	switch tag {
	case 5: // value.counter
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(CounterValue)
		err := b.DecodeMessage(msg)
		m.Value = &Metric_Counter{msg}
		return true, err
	case 6: // value.gauge
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(GaugeValue)
		err := b.DecodeMessage(msg)
		m.Value = &Metric_Gauge{msg}
		return true, err
	case 7: // value.histogram
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(HistogramValue)
		err := b.DecodeMessage(msg)
		m.Value = &Metric_Histogram{msg}
		return true, err
	case 8: // value.set
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(SetValue)
		err := b.DecodeMessage(msg)
		m.Value = &Metric_Set{msg}
		return true, err
	default:
		return false, nil
	}
}

func _Metric_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*Metric)
	// value
	switch x := m.Value.(type) {
	case *Metric_Counter:
		s := proto.Size(x.Counter)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Metric_Gauge:
		s := proto.Size(x.Gauge)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Metric_Histogram:
		s := proto.Size(x.Histogram)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *Metric_Set:
		s := proto.Size(x.Set)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

// CounterValue wraps the value of a counter
type CounterValue struct {
	Value int64 `protobuf:"varint,1,opt,name=value,proto3" json:"value,omitempty"`
}

func (m *CounterValue) Reset()         { *m = CounterValue{} }
func (m *CounterValue) String() string { return proto.CompactTextString(m) }
func (*CounterValue) ProtoMessage()    {}
func (*CounterValue) Descriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{1}
}
func (m *CounterValue) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *CounterValue) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_CounterValue.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *CounterValue) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CounterValue.Merge(m, src)
}
func (m *CounterValue) XXX_Size() int {
	return m.Size()
}
func (m *CounterValue) XXX_DiscardUnknown() {
	xxx_messageInfo_CounterValue.DiscardUnknown(m)
}

var xxx_messageInfo_CounterValue proto.InternalMessageInfo

func (m *CounterValue) GetValue() int64 {
	if m != nil {
		return m.Value
	}
	return 0
}

// GaugeValue wraps the value of a gauge
type GaugeValue struct {
	Value float64 `protobuf:"fixed64,1,opt,name=value,proto3" json:"value,omitempty"`
}

func (m *GaugeValue) Reset()         { *m = GaugeValue{} }
func (m *GaugeValue) String() string { return proto.CompactTextString(m) }
func (*GaugeValue) ProtoMessage()    {}
func (*GaugeValue) Descriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{2}
}
func (m *GaugeValue) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *GaugeValue) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_GaugeValue.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *GaugeValue) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GaugeValue.Merge(m, src)
}
func (m *GaugeValue) XXX_Size() int {
	return m.Size()
}
func (m *GaugeValue) XXX_DiscardUnknown() {
	xxx_messageInfo_GaugeValue.DiscardUnknown(m)
}

var xxx_messageInfo_GaugeValue proto.InternalMessageInfo

func (m *GaugeValue) GetValue() float64 {
	if m != nil {
		return m.Value
	}
	return 0
}

// HistogramValue for now just includes the t-digest.  This can be expanded
// to include the other values such as the sum, average, etc.
type HistogramValue struct {
	TDigest *tdigest.MergingDigestData `protobuf:"bytes,1,opt,name=t_digest,json=tDigest,proto3" json:"t_digest,omitempty"`
}

func (m *HistogramValue) Reset()         { *m = HistogramValue{} }
func (m *HistogramValue) String() string { return proto.CompactTextString(m) }
func (*HistogramValue) ProtoMessage()    {}
func (*HistogramValue) Descriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{3}
}
func (m *HistogramValue) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *HistogramValue) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_HistogramValue.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *HistogramValue) XXX_Merge(src proto.Message) {
	xxx_messageInfo_HistogramValue.Merge(m, src)
}
func (m *HistogramValue) XXX_Size() int {
	return m.Size()
}
func (m *HistogramValue) XXX_DiscardUnknown() {
	xxx_messageInfo_HistogramValue.DiscardUnknown(m)
}

var xxx_messageInfo_HistogramValue proto.InternalMessageInfo

func (m *HistogramValue) GetTDigest() *tdigest.MergingDigestData {
	if m != nil {
		return m.TDigest
	}
	return nil
}

// SetValue contains a binary-encoded HyperLogLog
type SetValue struct {
	HyperLogLog []byte `protobuf:"bytes,1,opt,name=hyper_log_log,json=hyperLogLog,proto3" json:"hyper_log_log,omitempty"`
}

func (m *SetValue) Reset()         { *m = SetValue{} }
func (m *SetValue) String() string { return proto.CompactTextString(m) }
func (*SetValue) ProtoMessage()    {}
func (*SetValue) Descriptor() ([]byte, []int) {
	return fileDescriptor_95975e4c0ef795ab, []int{4}
}
func (m *SetValue) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *SetValue) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_SetValue.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalTo(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *SetValue) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SetValue.Merge(m, src)
}
func (m *SetValue) XXX_Size() int {
	return m.Size()
}
func (m *SetValue) XXX_DiscardUnknown() {
	xxx_messageInfo_SetValue.DiscardUnknown(m)
}

var xxx_messageInfo_SetValue proto.InternalMessageInfo

func (m *SetValue) GetHyperLogLog() []byte {
	if m != nil {
		return m.HyperLogLog
	}
	return nil
}

func init() {
	proto.RegisterEnum("metricpb.Scope", Scope_name, Scope_value)
	proto.RegisterEnum("metricpb.Type", Type_name, Type_value)
	proto.RegisterType((*Metric)(nil), "metricpb.Metric")
	proto.RegisterType((*CounterValue)(nil), "metricpb.CounterValue")
	proto.RegisterType((*GaugeValue)(nil), "metricpb.GaugeValue")
	proto.RegisterType((*HistogramValue)(nil), "metricpb.HistogramValue")
	proto.RegisterType((*SetValue)(nil), "metricpb.SetValue")
}

func init() { proto.RegisterFile("samplers/metricpb/metric.proto", fileDescriptor_95975e4c0ef795ab) }

var fileDescriptor_95975e4c0ef795ab = []byte{
	// 446 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x54, 0x92, 0x41, 0x6b, 0xdb, 0x40,
	0x10, 0x85, 0xb5, 0x96, 0x65, 0x59, 0xe3, 0xc4, 0x15, 0x43, 0x5a, 0x96, 0x1c, 0x84, 0x11, 0x6d,
	0x71, 0x43, 0x51, 0xc0, 0xa5, 0xd0, 0x6b, 0xd3, 0x80, 0x73, 0xb0, 0x2f, 0x4a, 0xe8, 0x35, 0xac,
	0x9d, 0x65, 0x23, 0x90, 0xbc, 0x42, 0x5a, 0x97, 0xfa, 0x5f, 0xf4, 0x67, 0xe5, 0x98, 0x63, 0x8f,
	0xc5, 0xfe, 0x23, 0x65, 0x57, 0xda, 0x28, 0x3e, 0x08, 0xcd, 0xbe, 0xf7, 0x3d, 0x89, 0xa7, 0x11,
	0x44, 0x35, 0x2b, 0xca, 0x9c, 0x57, 0xf5, 0x65, 0xc1, 0x55, 0x95, 0xad, 0xcb, 0x55, 0x3b, 0x24,
	0x65, 0x25, 0x95, 0xc4, 0xa1, 0x95, 0xcf, 0xdf, 0xaa, 0x87, 0x4c, 0xf0, 0x5a, 0x5d, 0xb6, 0xf7,
	0x06, 0x88, 0x9f, 0x7a, 0x30, 0x58, 0x1a, 0x06, 0x11, 0xfa, 0x1b, 0x56, 0x70, 0x4a, 0x26, 0x64,
	0x1a, 0xa4, 0x66, 0xd6, 0x9a, 0x62, 0xa2, 0xa6, 0xbd, 0x89, 0xab, 0x35, 0x3d, 0x63, 0x0c, 0x7d,
	0xb5, 0x2b, 0x39, 0x75, 0x27, 0x64, 0x3a, 0x9e, 0x8d, 0x13, 0xfb, 0x8a, 0xe4, 0x6e, 0x57, 0xf2,
	0xd4, 0x78, 0x38, 0x03, 0x7f, 0x2d, 0xb7, 0x1b, 0xc5, 0x2b, 0xea, 0x4d, 0xc8, 0x74, 0x34, 0x7b,
	0xd7, 0x61, 0x3f, 0x1a, 0xe3, 0x27, 0xcb, 0xb7, 0xfc, 0xc6, 0x49, 0x2d, 0x88, 0x9f, 0xc1, 0x13,
	0x6c, 0x2b, 0x38, 0x1d, 0x98, 0xc4, 0x59, 0x97, 0x98, 0x6b, 0xd9, 0xf2, 0x0d, 0x84, 0xdf, 0x20,
	0x78, 0xcc, 0x6a, 0x25, 0x45, 0xc5, 0x0a, 0xea, 0x9b, 0x04, 0xed, 0x12, 0x37, 0xd6, 0xb2, 0xa9,
	0x0e, 0xc6, 0x8f, 0xe0, 0xd6, 0x5c, 0xd1, 0xa1, 0xc9, 0x60, 0x97, 0xb9, 0xe5, 0xca, 0xd2, 0x1a,
	0xc0, 0x0f, 0xe0, 0xd5, 0x6b, 0x59, 0x72, 0x1a, 0x98, 0xa2, 0x6f, 0x5e, 0x91, 0x5a, 0x4e, 0x1b,
	0xf7, 0xca, 0x07, 0xef, 0x97, 0x8e, 0xc5, 0xef, 0xe1, 0xe4, 0x75, 0x35, 0x3c, 0x6b, 0x0d, 0xf3,
	0x41, 0xdd, 0xb4, 0xa5, 0x62, 0x80, 0xae, 0xce, 0x31, 0x43, 0x2c, 0x33, 0x87, 0xf1, 0x71, 0x01,
	0xfc, 0x0a, 0x43, 0x75, 0xdf, 0x2c, 0xce, 0xa0, 0xa3, 0xd9, 0x79, 0x62, 0x17, 0xb9, 0xe4, 0x95,
	0xc8, 0x36, 0xe2, 0xda, 0x9c, 0xae, 0x99, 0x62, 0xa9, 0xaf, 0x9a, 0x43, 0x9c, 0xc0, 0xd0, 0xb6,
	0xc2, 0x18, 0x4e, 0x1f, 0x77, 0x25, 0xaf, 0xee, 0x73, 0x29, 0xf4, 0x65, 0x9e, 0x73, 0x92, 0x8e,
	0x8c, 0xb8, 0x90, 0x62, 0x21, 0xc5, 0xc5, 0x27, 0xf0, 0x4c, 0x37, 0x0c, 0xc0, 0x5b, 0x66, 0xbf,
	0xf9, 0x43, 0xe8, 0xe8, 0x71, 0x21, 0xd7, 0x2c, 0x0f, 0x09, 0x02, 0x0c, 0xe6, 0xb9, 0x5c, 0xb1,
	0x3c, 0xec, 0x5d, 0x7c, 0x87, 0xbe, 0xde, 0x37, 0x8e, 0xc0, 0x6f, 0x5b, 0x37, 0xac, 0x29, 0x17,
	0x12, 0x3c, 0x85, 0xe0, 0xa5, 0x43, 0xd8, 0x43, 0x1f, 0xdc, 0x5b, 0xae, 0x42, 0x57, 0x23, 0x77,
	0x59, 0xc1, 0xab, 0xb0, 0x7f, 0x45, 0x9f, 0xf6, 0x11, 0x79, 0xde, 0x47, 0xe4, 0xdf, 0x3e, 0x22,
	0x7f, 0x0e, 0x91, 0xf3, 0x7c, 0x88, 0x9c, 0xbf, 0x87, 0xc8, 0x59, 0x0d, 0xcc, 0xcf, 0xf9, 0xe5,
	0x7f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x50, 0xd7, 0x77, 0x2f, 0xdf, 0x02, 0x00, 0x00,
}

func (m *Metric) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Metric) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Name) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintMetric(dAtA, i, uint64(len(m.Name)))
		i += copy(dAtA[i:], m.Name)
	}
	if len(m.Tags) > 0 {
		for _, s := range m.Tags {
			dAtA[i] = 0x12
			i++
			l = len(s)
			for l >= 1<<7 {
				dAtA[i] = uint8(uint64(l)&0x7f | 0x80)
				l >>= 7
				i++
			}
			dAtA[i] = uint8(l)
			i++
			i += copy(dAtA[i:], s)
		}
	}
	if m.Type != 0 {
		dAtA[i] = 0x18
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Type))
	}
	if m.Value != nil {
		nn1, err := m.Value.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += nn1
	}
	if m.Scope != 0 {
		dAtA[i] = 0x48
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Scope))
	}
	return i, nil
}

func (m *Metric_Counter) MarshalTo(dAtA []byte) (int, error) {
	i := 0
	if m.Counter != nil {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Counter.Size()))
		n2, err := m.Counter.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n2
	}
	return i, nil
}
func (m *Metric_Gauge) MarshalTo(dAtA []byte) (int, error) {
	i := 0
	if m.Gauge != nil {
		dAtA[i] = 0x32
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Gauge.Size()))
		n3, err := m.Gauge.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n3
	}
	return i, nil
}
func (m *Metric_Histogram) MarshalTo(dAtA []byte) (int, error) {
	i := 0
	if m.Histogram != nil {
		dAtA[i] = 0x3a
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Histogram.Size()))
		n4, err := m.Histogram.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n4
	}
	return i, nil
}
func (m *Metric_Set) MarshalTo(dAtA []byte) (int, error) {
	i := 0
	if m.Set != nil {
		dAtA[i] = 0x42
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Set.Size()))
		n5, err := m.Set.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n5
	}
	return i, nil
}
func (m *CounterValue) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *CounterValue) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Value != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.Value))
	}
	return i, nil
}

func (m *GaugeValue) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *GaugeValue) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Value != 0 {
		dAtA[i] = 0x9
		i++
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.Value))))
		i += 8
	}
	return i, nil
}

func (m *HistogramValue) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *HistogramValue) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.TDigest != nil {
		dAtA[i] = 0xa
		i++
		i = encodeVarintMetric(dAtA, i, uint64(m.TDigest.Size()))
		n6, err := m.TDigest.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n6
	}
	return i, nil
}

func (m *SetValue) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SetValue) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.HyperLogLog) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintMetric(dAtA, i, uint64(len(m.HyperLogLog)))
		i += copy(dAtA[i:], m.HyperLogLog)
	}
	return i, nil
}

func encodeVarintMetric(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Metric) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Name)
	if l > 0 {
		n += 1 + l + sovMetric(uint64(l))
	}
	if len(m.Tags) > 0 {
		for _, s := range m.Tags {
			l = len(s)
			n += 1 + l + sovMetric(uint64(l))
		}
	}
	if m.Type != 0 {
		n += 1 + sovMetric(uint64(m.Type))
	}
	if m.Value != nil {
		n += m.Value.Size()
	}
	if m.Scope != 0 {
		n += 1 + sovMetric(uint64(m.Scope))
	}
	return n
}

func (m *Metric_Counter) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Counter != nil {
		l = m.Counter.Size()
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}
func (m *Metric_Gauge) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Gauge != nil {
		l = m.Gauge.Size()
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}
func (m *Metric_Histogram) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Histogram != nil {
		l = m.Histogram.Size()
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}
func (m *Metric_Set) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Set != nil {
		l = m.Set.Size()
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}
func (m *CounterValue) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Value != 0 {
		n += 1 + sovMetric(uint64(m.Value))
	}
	return n
}

func (m *GaugeValue) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Value != 0 {
		n += 9
	}
	return n
}

func (m *HistogramValue) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.TDigest != nil {
		l = m.TDigest.Size()
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}

func (m *SetValue) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.HyperLogLog)
	if l > 0 {
		n += 1 + l + sovMetric(uint64(l))
	}
	return n
}

func sovMetric(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozMetric(x uint64) (n int) {
	return sovMetric(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Metric) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Metric: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Metric: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Name = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Tags", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Tags = append(m.Tags, string(dAtA[iNdEx:postIndex]))
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= Type(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Counter", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			v := &CounterValue{}
			if err := v.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			m.Value = &Metric_Counter{v}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Gauge", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			v := &GaugeValue{}
			if err := v.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			m.Value = &Metric_Gauge{v}
			iNdEx = postIndex
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Histogram", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			v := &HistogramValue{}
			if err := v.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			m.Value = &Metric_Histogram{v}
			iNdEx = postIndex
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Set", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			v := &SetValue{}
			if err := v.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			m.Value = &Metric_Set{v}
			iNdEx = postIndex
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Scope", wireType)
			}
			m.Scope = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Scope |= Scope(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipMetric(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *CounterValue) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: CounterValue: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: CounterValue: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			m.Value = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Value |= int64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipMetric(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *GaugeValue) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: GaugeValue: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: GaugeValue: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 1 {
				return fmt.Errorf("proto: wrong wireType = %d for field Value", wireType)
			}
			var v uint64
			if (iNdEx + 8) > l {
				return io.ErrUnexpectedEOF
			}
			v = uint64(encoding_binary.LittleEndian.Uint64(dAtA[iNdEx:]))
			iNdEx += 8
			m.Value = float64(math.Float64frombits(v))
		default:
			iNdEx = preIndex
			skippy, err := skipMetric(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *HistogramValue) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: HistogramValue: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: HistogramValue: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field TDigest", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.TDigest == nil {
				m.TDigest = &tdigest.MergingDigestData{}
			}
			if err := m.TDigest.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMetric(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *SetValue) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: SetValue: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SetValue: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field HyperLogLog", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthMetric
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthMetric
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.HyperLogLog = append(m.HyperLogLog[:0], dAtA[iNdEx:postIndex]...)
			if m.HyperLogLog == nil {
				m.HyperLogLog = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipMetric(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthMetric
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipMetric(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowMetric
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowMetric
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthMetric
			}
			iNdEx += length
			if iNdEx < 0 {
				return 0, ErrInvalidLengthMetric
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowMetric
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipMetric(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
				if iNdEx < 0 {
					return 0, ErrInvalidLengthMetric
				}
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthMetric = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowMetric   = fmt.Errorf("proto: integer overflow")
)
