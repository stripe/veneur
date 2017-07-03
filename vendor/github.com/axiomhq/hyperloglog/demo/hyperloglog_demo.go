package main

import (
	"fmt"
	"strconv"

	"github.com/axiomhq/hyperloglog"
	"github.com/influxdata/influxdb/pkg/estimator/hll"
)

func estimateError(got, exp uint64) float64 {
	var delta uint64
	if got > exp {
		delta = got - exp
	} else {
		delta = exp - got
	}
	return float64(delta) / float64(exp)
}

func main() {
	axiom := hyperloglog.New16()
	influx, err := hll.NewPlus(14)
	if err != nil {
		panic(err)
	}

	step := 10
	unique := map[string]bool{}

	for i := 1; len(unique) <= 10000000; i++ {
		str := "stream-" + strconv.Itoa(i)
		axiom.Insert([]byte(str))
		influx.Add([]byte(str))
		unique[str] = true

		if len(unique)%step == 0 || len(unique) == 10000000 {
			step *= 5
			exact := uint64(len(unique))
			res := axiom.Estimate()
			ratio := 100 * estimateError(res, exact)
			res2 := influx.Count()
			ratio2 := 100 * estimateError(res2, exact)
			fmt.Printf("Exact %d, got:\n\t axiom HLL %d (%.4f%% off)\n\tinflux HLL++ %d (%.4f%% off)\n", exact, res, ratio, res2, ratio2)
		}
	}

	data1, err := influx.MarshalBinary()
	if err != nil {
		panic(err)
	}
	data2, err := axiom.MarshalBinary()
	if err != nil {
		panic(err)
	}
	fmt.Println("AxiomHQ HLL total size:\t", len(data2))
	fmt.Println("InfluxData HLL++ total size:\t", len(data1))

}
