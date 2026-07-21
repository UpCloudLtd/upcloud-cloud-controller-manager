package loadbalancer

type Config struct {
	clusterID        string
	clusterName      string
	loadBalancerPlan string
	// Maxium number of backend members to configure.
	maxBackendMembers int
}

func NewConfig(clusterID, clusterName, loadBalancerPlan string, maxBackendMembers int) *Config {
	if maxBackendMembers == 0 {
		maxBackendMembers = defaultMaxBackendMembers
	}
	return &Config{clusterID: clusterID, clusterName: clusterName, loadBalancerPlan: loadBalancerPlan, maxBackendMembers: maxBackendMembers}
}
