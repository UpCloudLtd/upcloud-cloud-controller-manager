package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"go.yaml.in/yaml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	apiUserEnv     string = "UPCLOUD_API_USER"
	apiPasswordEnv string = "UPCLOUD_API_PASSWORD"
)

type ConfigData struct {
	ClusterDeploymentMode string        `yaml:"clusterDeploymentMode"`
	APICredentials        Credentials   `yaml:"apiCredentials"`
	NodeScopeSelector     LabelSelector `yaml:"nodeScopeSelector"`
	ClusterName           string        `yaml:"clusterName"`
	ClusterID             string        `yaml:"clusterID"`

	LoadBalancerPlan              string `yaml:"loadBalancerPlan"`
	LoadBalancerMaxBackendMembers int    `yaml:"loadBalancerMaxBackendMembers"`

	nodeScopeLabelSelector labels.Selector
}

type Credentials struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type LabelSelector struct {
	MatchLabels      map[string]string          `yaml:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `yaml:"matchExpressions,omitempty"`
}

type LabelSelectorRequirement struct {
	Key      string                       `yaml:"key"`
	Operator metav1.LabelSelectorOperator `yaml:"operator"`
	Values   []string                     `yaml:"values,omitempty"`
}

type config struct {
	metav1.TypeMeta   `yaml:",inline"`
	metav1.ObjectMeta `yaml:"metadata,omitempty"`

	Data ConfigData `yaml:"data"`
}

func newConfig(f io.Reader) (config, error) {
	c := config{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Data: ConfigData{
			ClusterDeploymentMode: "",
			APICredentials: Credentials{
				User:     os.Getenv(apiUserEnv),
				Password: os.Getenv(apiPasswordEnv),
			},
			NodeScopeSelector:      LabelSelector{},
			ClusterName:            "",
			ClusterID:              "",
			LoadBalancerPlan:       loadbalancer.DefaultPlan,
			nodeScopeLabelSelector: labels.Everything(),
		},
	}

	if f == nil {
		return c, nil
	}

	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("can't read config file: %w", err)
	}

	// metav1.LabelSelector doesn't have YAML tags so we need to do some type juggling here.
	labelSelector, err := labelSelectorAsSelector(c.Data.NodeScopeSelector)
	if err != nil {
		return c, fmt.Errorf("can't parse nodeScopeSelector: %w", err)
	}

	c.Data.nodeScopeLabelSelector = labelSelector
	if c.Data.APICredentials.User == "" || c.Data.APICredentials.Password == "" {
		return c, errors.New("UpCloud user credentials not set")
	}
	return c, nil
}

func labelSelectorAsSelector(s LabelSelector) (labels.Selector, error) {
	me := make([]metav1.LabelSelectorRequirement, len(s.MatchExpressions))
	for i := range s.MatchExpressions {
		me[i] = metav1.LabelSelectorRequirement{
			Key:      s.MatchExpressions[i].Key,
			Operator: s.MatchExpressions[i].Operator,
			Values:   s.MatchExpressions[i].Values,
		}
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels:      s.MatchLabels,
		MatchExpressions: me,
	})
}
