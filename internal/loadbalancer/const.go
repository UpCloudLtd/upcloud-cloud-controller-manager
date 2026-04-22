package loadbalancer

const (
	DefaultPlan string = "development"

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
