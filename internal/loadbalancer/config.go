package loadbalancer

type Config struct {
	clusterID        string
	clusterName      string
	loadBalancerPlan string
	// Maxium number of backend members to configure.
	maxBackendMembers int
	// Where a backend member's address is taken from; one of
	// BackendAddressSourceNodeInternalIP (default) or
	// BackendAddressSourcePrivateNetwork.
	backendAddressSource string
}

func NewConfig(clusterID, clusterName, loadBalancerPlan string, maxBackendMembers int, backendAddressSource string) *Config {
	if maxBackendMembers == 0 {
		maxBackendMembers = defaultMaxBackendMembers
	}
	if backendAddressSource == "" {
		backendAddressSource = BackendAddressSourceNodeInternalIP
	}
	return &Config{
		clusterID:            clusterID,
		clusterName:          clusterName,
		loadBalancerPlan:     loadBalancerPlan,
		maxBackendMembers:    maxBackendMembers,
		backendAddressSource: backendAddressSource,
	}
}
