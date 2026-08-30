package loadbalancer

const (
	DefaultPlan string = "development"

	// BackendAddressSourceNodeInternalIP builds backend members from the
	// node's Kubernetes InternalIP. This is the default and what the
	// controller has always done.
	BackendAddressSourceNodeInternalIP string = "node-internal-ip"

	// BackendAddressSourcePrivateNetwork builds backend members from the
	// address the node's server holds on the SDN private network the load
	// balancer itself is attached to.
	//
	// A managed load balancer reaches its backends over that private
	// network, so the node's address on it is the only address guaranteed
	// to be routable from the load balancer. The InternalIP is normally
	// the same address, but it need not be: a cluster that advertises an
	// overlay address (a WireGuard mesh spanning zones, for example) as
	// the node IP leaves every member unreachable, and because the API
	// accepts the members without complaint the service is created,
	// reports success and then never passes a health check.
	BackendAddressSourcePrivateNetwork string = "private-network"

	// loadBalancerIDAnnotation is the annotation specifying the load balancer ID
	// used to enable fast retrievals of load balancers from the API by UUID.
	loadBalancerIDAnnotation = "service.beta.kubernetes.io/upcloud-load-balancer-id"

	// loadBalancerNameAnnotation is the annotation specifying the load balancer name.
	loadBalancerNameAnnotation = "service.beta.kubernetes.io/upcloud-load-balancer-name"

	// loadBalancerConfigAnnotation is the annotation specifying the load balancer configuration.
	// in json format. For available fields consult readme
	// (based on the upstream API, but not all fields can be configured:
	// https://developers.upcloud.com/1.3/17-managed-loadbalancer/#create-service)
	loadBalancerConfigAnnotation = "service.beta.kubernetes.io/upcloud-load-balancer-config"

	// loadBalancerNodeSelectorAnnotation is the annotation specifying the load balancer nodes selector
	// for backend members.
	loadBalancerNodeSelectorAnnotation = "service.beta.kubernetes.io/upcloud-load-balancer-node-selector"

	// clusterIDLabel is a key for label that should store owner cluster ID as a value.
	clusterIDLabel = "ccm_cluster_id"

	// clusterNameLabel is a key for label that should store owner cluster name as a value.
	clusterNameLabel = "ccm_cluster_name"

	// generatedNameLabel is a key for label that should store the unique (generated) name of a resource.
	generatedNameLabel = "ccm_generated_name"

	// ServiceExternalTrafficPolicyLabelKey is a key for label that should store external traffic policy type as a value.
	serviceExternalTrafficPolicyLabel string = "ccm_external_traffic_policy"

	changesDetectedEventType   string = "ChangesDetected"
	noChangesDetectedEventType string = "NoChangesDetected"
	newLoadBalancerEventType   string = "NewLoadBalancer"
	nodeCountLimitReached      string = "NodeCountLimitReached"

	loadBalancerNameMaxLength int = 64
	loadBalancerIDMaxLength   int = 36
	defaultMaxBackendMembers  int = 100

	certificateBundleNameMaxLength int    = 64
	certificateBundleNameSuffix    string = "-tls"
	needsCertificateToken          string = "needs-certificate"
)
