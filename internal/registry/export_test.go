package registry

// CgroupViewForTest exposes the server's CPU resolution over a caller-supplied
// cgroup layout, together with the integer budget the server would plan from
// there, so the validation host contract can be exercised against synthetic
// layouts through the same code the server plans with. Test-only: the
// production entry points (EffectiveCPUAllowance, EffectiveNumCPU) always read
// the real mount.
var CgroupViewForTest = func(root, membership string, scheduler, affinity int) (CPUAllowance, int) {
	quota, source := readCurrentCPUQuota(root, membership)
	return resolveCPUAllowance(quota, source, scheduler, affinity), readCgroupCPUQuota(root)
}
