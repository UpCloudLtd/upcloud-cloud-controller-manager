package main

import (
	"k8s.io/component-base/cli/flag"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/cloud"

	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
)

func main() {
	opts, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("failed to construct options: %v", err)
	}

	opts.KubeCloudShared.CloudProvider.Name = cloud.ProviderName
	opts.Authentication.SkipInClusterLookup = true
	controllerAliases := names.CCMControllerAliases()
	command := app.NewCloudControllerManagerCommand(
		opts,
		cloudInitializer,
		app.DefaultInitFuncConstructors,
		controllerAliases,
		flag.NamedFlagSets{},
		wait.NeverStop,
	)

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		klog.Fatalf("command exited with error: %v", err)
	}
}

func cloudInitializer(cfg *config.CompletedConfig) cloudprovider.Interface {
	cloudConfig := cfg.ComponentConfig.KubeCloudShared.CloudProvider

	// initialize cloud provider with the cloud provider name and config file provided
	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil || cloud == nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}

	if !cloud.HasClusterID() {
		if cfg.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("detected a cluster without a ClusterID. A ClusterID will be required in the future." +
				" Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatalf("no ClusterID found. A ClusterID is required for the cloud provider to function properly." +
				" This check can be bypassed by setting the allow-untagged-cloud option")
		}
	}

	return cloud
}
