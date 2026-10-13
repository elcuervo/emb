package registry

// CgroupViewForTest exposes the server's CPU resolution over a caller-supplied
// mountinfo and cgroup membership file, together with the integer capacity the
// server would plan from there, so the validation host contract can be
// exercised against synthetic layouts through the same code the server plans
// with. Test-only: the production entry points (EffectiveCPUAllowance,
// EffectiveNumCPU) always read the real /proc files.
var CgroupViewForTest = func(mountinfo, membership string, scheduler, affinity int) (CPUAllowance, int) {
	allowance := resolveCPU(mountinfo, membership, scheduler, affinity)
	return allowance, allowance.Capacity()
}
