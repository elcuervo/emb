//go:build !darwin && !linux

package registry

func TotalSystemMemory() uint64 {
	return 0
}
