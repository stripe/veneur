package fastrand

var globalPool = NewRandPool()

func Int63() int64 {
	return globalPool.Int63()
}

func Float64() float64 {
	return globalPool.Float64()
}
