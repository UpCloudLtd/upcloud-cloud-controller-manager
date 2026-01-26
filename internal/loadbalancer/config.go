package loadbalancer

type Config struct {
	clusterID        string
	loadBalancerPlan string
	// Maxium number of backend members to configure.
	maxBackendMembers int
}

func NewConfig(clusterID, loadBalancerPlan string, maxBackendMembers int) *Config {
	if maxBackendMembers == 0 {
		maxBackendMembers = defaultMaxBackendMembers
	}
	return &Config{clusterID: clusterID, loadBalancerPlan: loadBalancerPlan, maxBackendMembers: maxBackendMembers}
}
