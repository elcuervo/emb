//go:build !darwin && !linux

package registry

func totalSystemMemory() uint64 {
	return 0
}
