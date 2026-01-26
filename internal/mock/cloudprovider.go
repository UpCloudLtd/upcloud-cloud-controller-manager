package mock

import (
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	restclient "k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
)

type controllerClientBuilder struct{}

func (c *controllerClientBuilder) Config(_ string) (*restclient.Config, error) {
	return &restclient.Config{}, nil
}

func (c *controllerClientBuilder) ConfigOrDie(_ string) *restclient.Config {
	return nil
}

func (c *controllerClientBuilder) Client(_ string) (clientset.Interface, error) {
	return fake.NewSimpleClientset(), nil
}

func (c *controllerClientBuilder) ClientOrDie(_ string) clientset.Interface {
	return fake.NewSimpleClientset()
}

func NewControllerClientBuilder() cloudprovider.ControllerClientBuilder {
	return &controllerClientBuilder{}
}
