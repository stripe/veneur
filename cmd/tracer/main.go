package main

import (
	"fmt"
	"log"
	"net"

	"github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/ssf"
)

func main() {
	tags := []*ssf.SSFTag{}
	tags = append(tags, &ssf.SSFTag{Name: *proto.String("foo"), Value: *proto.String("bar")})
	tags = append(tags, &ssf.SSFTag{Name: *proto.String("baz")})

	test := &ssf.SSFSample{
		Metric:    ssf.SSFSample_TRACE.Enum(),
		Timestamp: *proto.Int64(1474599325),
		Status:    ssf.SSFSample_OK.Enum(),
		Name:      *proto.String("foo.bar"),
		// Message:   proto.String("Hello World!"),
		Trace: &ssf.SSFTrace{
			TraceId:  *proto.Int64(1234),
			Id:       *proto.Int64(1235),
			Duration: *proto.Int64(12),
			ParentId: *proto.Int64(23),
		},
		Value:      *proto.Float64(1.234),
		SampleRate: *proto.Float32(0.50),
		Tags:       tags,
		// Unit:       proto.String("farts"),
	}
	data, err := proto.Marshal(test)
	if err != nil {
		log.Fatal("marshaling error: ", err)
	}

	server_addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:8128")
	local_addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")

	conn, err := net.DialUDP("udp", local_addr, server_addr)

	defer conn.Close()

	buf := []byte(data)
	_, err = conn.Write(buf)
	if err != nil {
		log.Fatal("error sending span: ", err)
	}

	// fmt.Printf("Data: %s\n", string(data[:len(data)]))
	// fmt.Println(reflect.TypeOf(data))

	newSample := &ssf.SSFSample{}
	err = proto.Unmarshal(data, newSample)
	if err != nil {
		log.Fatal("unmarshaling error: ", err)
	}
	fmt.Printf("%s\n", proto.CompactTextString(newSample))
}
