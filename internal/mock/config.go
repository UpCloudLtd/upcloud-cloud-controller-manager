package mock

import (
	"io"
	"strings"
)

func ConfigReader() io.Reader {
	const testConfig string = `
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: CCMConfig
metadata:
  name: ccm-config
data:
  clusterName: test-cluster
  clusterID: 0de1e792-2f7c-40df-b1d4-38c0a5a5400b
  loadBalancerPlan: "development"
  loadBalancerMaxBackendMembers: 3
  apiCredentials:
    user: "testuser"
    password: "testpass"
  nodeScopeSelector:
    matchExpressions:
      - { key: node-role.kubernetes.io/control-plane, operator: DoesNotExist }
      - { key: node-role.kubernetes.io/master, operator: DoesNotExist }
      - { key: node.kubernetes.io/exclude-from-external-load-balancers, operator: DoesNotExist}
`
	return strings.NewReader(testConfig)
}
