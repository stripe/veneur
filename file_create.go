package veneur

// Change to `package main` to be able to run this file with `go run file_create.go`

// import (
// 	"fmt"
// 	"os"
// 	"path/filepath"

// 	"github.com/sirupsen/logrus"
// 	"github.com/gogo/protobuf/proto"
// 	"github.com/stripe/veneur/v14/ssf"
// )

// func check(err error) {
// 	if err != nil {
// 		logrus.WithError(err).Fatal("rip")
// 	}
// }

// func main() {
// 	span := &ssf.SSFSpan{
// 		TraceId:        1,
// 		Id:             1,
// 		StartTimestamp: 1,
// 		EndTimestamp:   10,
// 		Error:          false,
// 		Service:        "testService",
// 		Operation:      "operationTest",
// 		Tags:           map[string]string{"tag1": "value1"},
// 	}
// 	buf, _ := proto.Marshal(span)
// 	fmt.Println(buf)

// 	pbFile := filepath.Join("fixtures", "protobuf", "regression.pb")
// 	fmt.Println(pbFile)
// 	file, err := os.Create(pbFile)
// 	defer file.Close()
// 	check(err)

// 	var n int
// 	n, err = file.Write(buf)
// 	check(err)
// 	fmt.Printf("n: %d buf: %d\n", n, len(buf))
// }
