package loadbalancer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// const autoTLS string = "needs-certificate"

var (
	errNotFound  = errors.New("not found")
	errAmbiguity = errors.New("ambiguity")
)

func loadBalancerIDFromService(service *v1.Service) uuid.UUID {
	annotations := service.GetAnnotations()
	if annotations == nil {
		return uuid.Nil
	}
	lbID, ok := annotations[loadBalancerIDAnnotation]
	if !ok {
		return uuid.Nil
	}
	if v, err := uuid.Parse(lbID); err == nil {
		return v
	}
	return uuid.Nil
}

func queryFilterToString(f []request.QueryFilter) string {
	u := url.Values{}
	for _, v := range f {
		p, err := url.ParseQuery(v.ToQueryParam())
		if err == nil {
			for key := range p {
				u.Add(key, p.Get(key))
			}
		}
	}
	return u.Encode()
}

func selectNodesByLabels(nodes []*v1.Node, selector labels.Selector) []*v1.Node {
	if selector == nil {
		return nodes
	}
	n := make([]*v1.Node, 0)
	for i := range nodes {
		if selector.Matches(labels.Set(nodes[i].Labels)) {
			n = append(n, nodes[i])
		}
	}
	return n
}

func nodeSelectorFromService(service *v1.Service) (labels.Selector, error) {
	annotations := service.GetAnnotations()
	if annotations == nil {
		return labels.Everything(), nil
	}
	selectorStr, ok := annotations[loadBalancerNodeSelectorAnnotation]
	if !ok {
		return labels.Everything(), nil
	}

	s := metav1.LabelSelector{}
	if err := yaml.Unmarshal([]byte(selectorStr), &s); err != nil {
		return nil, fmt.Errorf("can't unmarshal label selector from %s: %w", selectorStr, err)
	}

	return metav1.LabelSelectorAsSelector(&s)
}

func updateServiceAnnotations(service *v1.Service, lb *upcloud.LoadBalancer) {
	if lb == nil || service == nil {
		return
	}
	annotations := service.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[loadBalancerIDAnnotation] = lb.UUID
	annotations[loadBalancerNameAnnotation] = lb.Name
	service.Annotations = annotations
}

func serviceHasAnnotation(service *v1.Service, annotation string) bool {
	annotations := service.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, ok := annotations[annotation]
	return ok
}

func loadBalancerFrontendFromServicePort(p *v1.ServicePort) request.LoadBalancerFrontend {
	portName := servicePortName(p)
	f := false
	return request.LoadBalancerFrontend{
		Name:           portName,
		Mode:           upcloud.LoadBalancerModeHTTP,
		Port:           int(p.Port),
		DefaultBackend: portName,
		Rules:          make([]request.LoadBalancerFrontendRule, 0),
		Networks: []upcloud.LoadBalancerFrontendNetwork{
			{Name: networkNamePublic},
			{Name: networkNamePrivate},
		},
		Properties: &upcloud.LoadBalancerFrontendProperties{
			TimeoutClient:        10,
			InboundProxyProtocol: nil,
			HTTP2Enabled:         &f,
		},
	}
}

func loadBalancerBackendFromServicePort(p *v1.ServicePort, nodes []*v1.Node, plan upcloud.LoadBalancerPlan) request.LoadBalancerBackend {
	portName := servicePortName(p)
	b := request.LoadBalancerBackend{
		Name:     portName,
		Resolver: "",
		Members:  make([]request.LoadBalancerBackendMember, 0, len(nodes)),
	}
	for _, node := range nodes {
		m := request.LoadBalancerBackendMember{
			Name:        node.Name,
			Weight:      100,
			MaxSessions: plan.PerServerMaxSessions,
			Enabled:     true,
			Type:        upcloud.LoadBalancerBackendMemberTypeStatic,
			IP:          "",
			Port:        int(p.NodePort),
		}
		for _, addr := range node.Status.Addresses {
			if addr.Type == v1.NodeInternalIP {
				m.IP = addr.Address
				b.Members = append(b.Members, m)
			}
		}
	}
	f := false
	b.Properties = &upcloud.LoadBalancerBackendProperties{
		TimeoutServer:             10,
		TimeoutTunnel:             3600,
		HealthCheckType:           upcloud.LoadBalancerHealthCheckTypeTCP,
		HealthCheckInterval:       10,
		HealthCheckFall:           3,
		HealthCheckRise:           3,
		HealthCheckURL:            "/",
		HealthCheckExpectedStatus: 200,
		StickySessionCookieName:   "",
		OutboundProxyProtocol:     "",
		TLSEnabled:                &f,
		TLSVerify:                 &f,
		TLSUseSystemCA:            &f,
		HTTP2Enabled:              &f,
		HealthCheckTLSVerify:      &f,
	}

	return b
}

func servicePortName(p *v1.ServicePort) string {
	if p.Name == "" {
		return fmt.Sprintf("config_%d", p.Port)
	}
	return p.Name
}

func serviceHealthCheckURL(service *v1.Service) string {
	if service.Spec.HealthCheckNodePort > 0 {
		return fmt.Sprintf("http://:%d", service.Spec.HealthCheckNodePort)
	}
	return "/"
}

func loadBalancerName(namespace, name, prefix string) string {
	var n string
	if prefix != "" {
		// 'p' contains clusterID or clusterName which can be shortened to 36 chars (loadBalancerIDMaxLength).
		// That leaves 13 characters for each namespace and name, plus two dashes to separate the name components.
		if len(prefix) < loadBalancerIDMaxLength {
			// try to use maximum available space for namespace if prefix is something else than UUID (loadBalancerIDMaxLength)
			prefix = fmt.Sprintf("%s-%s-", prefix, shortenString(namespace, (loadBalancerNameMaxLength-len(prefix)-2)/2))
		} else {
			prefix = fmt.Sprintf("%s-%s-", shortenString(prefix, loadBalancerIDMaxLength), shortenString(namespace, 13))
		}
		n = prefix + shortenString(name, loadBalancerNameMaxLength-len(prefix))
	} else {
		ns := shortenString(namespace, 31)
		n = fmt.Sprintf("%s-%s", ns, shortenString(name, loadBalancerNameMaxLength-len(ns)-1))
	}
	return n
}

func shortenString(s string, length int) string {
	if len(s) > length {
		return s[:length]
	}
	return s
}

func mergeLoadBalancerConfigFromServiceAnnotations(service *v1.Service, r *request.CreateLoadBalancerRequest, plans map[string]upcloud.LoadBalancerPlan) (err error) {
	annotations := service.GetAnnotations()
	if annotations == nil {
		return nil
	}
	config, ok := annotations[loadBalancerConfigAnnotation]
	if !ok {
		return nil
	}

	// Copy defaults before applying user config to `r`, so that we can reference them later.
	defaults := *r

	// Reset default slices because ordering might differ between service spec and what user has defined causing unexpected results.
	r.Backends = make([]request.LoadBalancerBackend, 0)
	r.Frontends = make([]request.LoadBalancerFrontend, 0)
	r.Labels = make([]upcloud.Label, 0, len(defaults.Labels))
	r.Networks = make([]request.LoadBalancerNetwork, 0)
	if err := json.Unmarshal([]byte(config), &r); err != nil {
		return fmt.Errorf("%w: can't parse annotations[%s], got error: '%v'", errUnsupportedConfiguration, loadBalancerConfigAnnotation, err)
	}

	if r.Plan != defaults.Plan {
		// Update defaults if non-default plan is selected.
		plan, ok := plans[r.Plan]
		if !ok {
			return fmt.Errorf("%s is not valid plan", r.Plan)
		}
		updateCreateLoadBalancerRequestPlan(&defaults, plan)
	}

	r.Backends = mergeLoadBalancerBackends(r.Backends, defaults.Backends, service)
	if r.Frontends, err = mergeLoadBalancerFrontends(r.Frontends, defaults.Frontends); err != nil {
		return err
	}
	if r.Labels == nil {
		r.Labels = make([]upcloud.Label, 0)
	}
	if err = validateUserDefinedLabels(r.Labels); err != nil {
		return err
	}
	r.Labels = append(r.Labels, defaults.Labels...)

	if len(r.Networks) == 0 {
		r.Networks = defaults.Networks
	}
	return nil
}

func mergeLoadBalancerBackends(backends, defaults []request.LoadBalancerBackend, service *v1.Service) []request.LoadBalancerBackend {
	if len(backends) == 0 {
		return defaults
	}
	// For backward compatibility, we need to support single port configs without custom backend name being defined.
	if len(backends) == 1 && len(defaults) == 1 && backends[0].Name == "" {
		backends[0].Name = defaults[0].Name
	}
	for i, backend := range backends {
		for _, defaultBackend := range defaults {
			if backend.Name == defaultBackend.Name {
				if backend.Resolver == "" {
					backends[i].Resolver = defaultBackend.Resolver
				}
				if len(backend.Members) == 0 {
					backends[i].Members = defaultBackend.Members
				}
				if backend.Properties == nil {
					backends[i].Properties = defaultBackend.Properties
				}
				break
			}
		}
		if backends[i].Properties != nil {
			sanitizeBackendProperties(service, backends[i].Properties)
		}
	}
	return backends
}

func mergeLoadBalancerFrontends(frontends, defaults []request.LoadBalancerFrontend) ([]request.LoadBalancerFrontend, error) {
	if len(frontends) == 0 {
		return defaults, nil
	}
	// for backward compatibility, we need to support single port configs without custom frontend name being defined
	if len(frontends) == 1 && len(defaults) == 1 && frontends[0].Name == "" {
		frontends[0].Name = defaults[0].Name
	}
	for i, frontend := range frontends {
		for _, defaultFrontend := range defaults {
			if frontend.Name == defaultFrontend.Name {
				if frontend.Mode == "" {
					frontends[i].Mode = defaultFrontend.Mode
				}
				if frontend.Port == 0 {
					frontends[i].Port = defaultFrontend.Port
				}
				if frontend.DefaultBackend == "" {
					frontends[i].DefaultBackend = defaultFrontend.DefaultBackend
				}
				if len(frontend.Rules) == 0 {
					frontends[i].Rules = defaultFrontend.Rules
				}
				if frontend.TLSConfigs == nil && frontend.Mode == upcloud.LoadBalancerModeHTTP {
					frontends[i].TLSConfigs = defaultFrontend.TLSConfigs
				}
				if frontend.Properties == nil {
					frontends[i].Properties = defaultFrontend.Properties
				}
				if len(frontend.Networks) == 0 && len(defaultFrontend.Networks) > 0 {
					frontends[i].Networks = defaultFrontend.Networks
				}
				break
			}
		}

		for j, rule := range frontend.Rules {
			for _, action := range rule.Actions {
				if action.Type == upcloud.LoadBalancerActionTypeUseBackend {
					return frontends, fmt.Errorf("%s rule item %d, action %s is not allowed in context of managed load balancer; %w", frontend.Name, j, action.Type, errUnsupportedConfiguration)
				}
			}
		}
	}
	return frontends, nil
}

func sanitizeBackendProperties(service *v1.Service, properties *upcloud.LoadBalancerBackendProperties) {
	if service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal {
		if properties.HealthCheckURL == "" && properties.HealthCheckType == "" {
			properties.HealthCheckType = upcloud.LoadBalancerHealthCheckTypeHTTP
			properties.HealthCheckURL = serviceHealthCheckURL(service)
		}
	}
}

func validateUserDefinedLabels(labels []upcloud.Label) error {
	if len(labels) == 0 {
		return nil
	}

	forbiddenKeys := map[string]bool{
		clusterIDLabel:                    true,
		clusterNameLabel:                  true,
		generatedNameLabel:                true,
		serviceExternalTrafficPolicyLabel: true,
	}
	for i := range labels {
		if _, ok := forbiddenKeys[labels[i].Key]; ok {
			return fmt.Errorf("faild to validate labels, forbidden label key '%s' defined", labels[i].Key)
		}
	}
	return nil
}

func loadBalancerCertBundleName(loadBalancerName string) string {
	if loadBalancerName == "" {
		// this shouldn't happen but return valid name just in case
		return "certificate-bundle" + certificateBundleNameSuffix
	}
	if len(loadBalancerName)+len(certificateBundleNameSuffix) > certificateBundleNameMaxLength {
		return shortenString(loadBalancerName, certificateBundleNameMaxLength-len(certificateBundleNameSuffix)) + certificateBundleNameSuffix
	}
	return loadBalancerName + certificateBundleNameSuffix
}

// errorAsProblem tries to convert error to upcloud.Problem if possible.
func errorAsProblem(err error) error {
	var errClient *client.Error
	if !errors.As(err, &errClient) {
		return err
	}
	if errClient.Type != client.ErrorTypeProblem {
		return err
	}
	p := &upcloud.Problem{}
	if err := json.Unmarshal(errClient.ResponseBody, p); err != nil {
		return err
	}
	return p
}

func loadBalancerGeneratedName(lb *upcloud.LoadBalancer) string {
	if n := generatedNameLabelValue(lb.Labels); n != "" {
		return n
	}
	return lb.Name
}

func generatedNameLabelValue(labels []upcloud.Label) string {
	if len(labels) > 0 {
		for i := range labels {
			if labels[i].Key == generatedNameLabel {
				return labels[i].Value
			}
		}
	}
	return ""
}

func loadBalancerDNSName(lb *upcloud.LoadBalancer) string {
	// TODO: lb.DNSName is deprecated, use lb.Networks slice to resolve DNS name.
	return lb.DNSName
}

func loadBalancerPrivateNetwork(lb *upcloud.LoadBalancer) (upcloud.LoadBalancerNetwork, error) {
	for i := range lb.Networks {
		if lb.Networks[i].Name == networkNamePrivate && lb.Networks[i].Type == upcloud.LoadBalancerNetworkTypePrivate {
			return lb.Networks[i], nil
		}
	}
	for i := range lb.Networks {
		if lb.Networks[i].Type == upcloud.LoadBalancerNetworkTypePrivate {
			return lb.Networks[i], nil
		}
	}
	return upcloud.LoadBalancerNetwork{}, errors.New("unable to find load balancer's private network interface")
}

func certificateAutoProvisioningNeeded(fe request.LoadBalancerFrontend) bool {
	if fe.Mode != upcloud.LoadBalancerModeHTTP {
		return false
	}
	if fe.Port == 443 && len(fe.TLSConfigs) == 0 {
		return true
	}
	for i := range fe.TLSConfigs {
		if fe.TLSConfigs[i].Name == needsCertificateToken {
			return true
		}
	}
	return false
}

func errIsHTTPStatusConflict(err error) bool {
	if err != nil {
		var p *upcloud.Problem
		return errors.As(err, &p) && p.Status == http.StatusConflict
	}
	return false
}
