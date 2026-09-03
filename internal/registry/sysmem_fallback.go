//go:build !darwin && !linux

package registry

func TotalSystemMemory() uint64 {
	return 0
}

// CurrentMemoryUsage falls back to the Go heap on platforms without a
// resident-set-size source (fromRSS=false, per the resource-usage-reporting
// spec).
func CurrentMemoryUsage() (bytes uint64, fromRSS bool) {
	return HeapInUseBytes(), false
}
