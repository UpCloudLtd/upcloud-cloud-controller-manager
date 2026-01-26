# Kubernetes cloud controller manager for UpCloud
## How to run
### Outside Kubernetes (developer mode):
```shell
export UPCLOUD_API_USER='valid upcloud user name'
export UPCLOUD_API_PASSWORD='user password'
make manager
./bin/cloud-controller-manager \
--allow-untagged-cloud \
--kubeconfig='/path/to/your/kubeconfig' \
-v 3 \
--configure-cloud-routes='false'
```

### Inside Kubernetes (standard mode)
Cloud controller manager can be deployed as single replica deployment using configmap and secret to store config data and credentials.

*TBD: add example deployment manifest to this repository*

## General concepts
Current implementation of ccm, covers 2 aspects - node controller and load-balancer controller. (Here you can find more details: https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
 - Node controller runs specific logic for node boostraping process after new node joined the k8s cluster. So ccm is critical for k8s cluster boostrpaing process.
After the first k8s node in cluster started with (`kubeadm init` command), ccm must be deployed into k8s cluster to proper boostrapping of the node.
 - Service controller allows to deploy load-balancer for k8s `service` object.
 - Node in scope of CCM for node and service controllers are defined by nodeScopeSelector, defined in CCM configuration file (see below).

## Configuration file
The configuration file set by specifying `--cloud-config=path/to/config.yaml` option.
The cconfiguration file format is:
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: CCMConfig
metadata:
  name: ccm-config
data:
  clusterName: ""
  clusterID: ""
  loadBalancerPlan: ""
  loadBalancerMaxBackendMembers: 100
  nodeScopeSelector: #Describe selector that defines the scope of the nodes, that CCM will operate on
                     #https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
    matchExpressions:
      - { key: node-role.kubernetes.io/control-plane, operator: DoesNotExist}
      - { key: node-role.kubernetes.io/master, operator: DoesNotExist}
      - { key: node.kubernetes.io/exclude-from-external-load-balancers, operator: DoesNotExist}
```

## Node controller
Limitation:
 Node must:
 - be attached to sing private network (attachment in multiple private networks not supported currently)
 - have annotation `infrastructure.cluster.x-k8s.io/upcloud-vm-uuid` with VM UUID, (set as post kubeadm command)
  
Essentially, when node controller bootstraps the node it sets: 
 - `porviderID` in the following format: upclod:////VM_UUID
 - `PrivateIP` with the private network IP, this IP is what is used for communication inside k8s cluster
 - `PublicIP` with the public network IP (if exist)
 - `infrastructure.cluster.x-k8s.io/upcloud-vm-private-nw-uuid` annotation that holds private network UUID of VM running node
If node is bootstrapped successfully, then special taints removed from the node, that allows normal scheduling process (as specified [here](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)).

## Service controller
### General concepts
Load balancer provisioned based on the service spec. For every item in `service.spec.ports` array a pair of front end and backend created
to allow to serve several ports per load-balancer. Each fronted/backend can be configured through annotations (see below),
configuration matching based on:
 - for `frontends`: `name` and `port` in `service.beta.kubernetes.io/upcloud-loadbalancer-config` should match to `service.spec.ports` item. Then the configuration will be applied for this frontend
 - for `backends`: `name` in `service.beta.kubernetes.io/upcloud-loadbalancer-config` should match to `service.spec.ports` item. Then the configuration will be applied for this backend

### Configuration
Load balancers can be configured with annotations.
1. `service.beta.kubernetes.io/upcloud-load-balancer-config`, defines the whole LB config, the format and description below:
```json
{
    "name": "specify here you custom LB name to override service.name",
    "plan": "development", // https://developers.upcloud.com/1.3/17-managed-loadbalancer/#list-plans
     // Please NOTE that there are reserved labels keys: ccm_cluster_id, ccm_cluster_name, ccm_generated_name
     // If labels with those keys will be specified below, they will be ignored and controller will inject it's own value.
    "labels": [ // https://developers.upcloud.com/1.3/17-managed-loadbalancer/#service-labels-usage-examples
        {
          "key": "env",
          "value": "staging"
        },
        {
          "key": "foo",
          "value": "bar"
        }
    ], 
    "frontends": [
        {
            "name": "must match port name in spec.ports",
            "mode": "http", // 'http' or 'tcp': when load balancer operating in tcp mode it acts as a layer 4 proxy. In http mode it acts as a layer 7 proxy.
            "port": 443, // Must match to service.spec.ports[x].port
            // Description: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#create-rule
            "rules": [
              {
               "name": "example-rule-1",
               "priority": 100,
               "matchers": [
                {
                 "type": "path",
                 "match_path": {
                  "method": "exact",
                  "value": "/app"
                 }
                }
               ],
               "actions": [ // All actions except 'use_backend' supported
                {
                  "type": "tcp_reject",
                  "action_tcp_reject": {}
                }
               ]
              }
             ],
         
            // Description: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#create-tls-config
            "tls_configs": [
                {
                    "name": "example-tls-config",
                    "certificate_bundle_uuid": "0aded5c1-c7a3-498a-b9c8-a871611c47a2"
                }
            ],

           // Description: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#frontend-properties
            "properties": {
                "timeout_client": 5,
                "inbound_proxy_protocol": false
            }
        }
    ],
    "backends": [
        {
          "name": "must match port name in spec.ports",
          // you can't define "members" and "resolver" here as
          // backend members are injected automatically (worker nodes, with label selector)
          
          // Description: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#backend-properties  
          "properties": {
                "timeout_server": 10,
                "timeout_tunnel": 3600,
                "outbound_proxy_protocol": "",
                "health_check_type": "http",
                "health_check_interval": 10,
                "health_check_fall": 5,
                "health_check_rise": 5,
                "health_check_url": "/health",
                "health_check_expected_status": 200,
                "sticky_session_cookie_name": "x-session"
            }
        }
    ]
}
```

Note that, when using local external traffic policy, you should leave `health_check_type` and `health_check_url` undefined and use default values. 
Default HTTP health check URL is generated based on `healthCheckNodePort` value. 

2. `service.beta.kubernetes.io/upcloud-load-balancer-node-selector` - defines node label selector to be applied on top of CCM nodes scope (see configuration file section).
The resulting list of nodes would be added automatically as load-balancer backend members for each backend.
The format is string representation of labels selector object (yaml or json notation supported):
```yaml 
matchLabels:
  kubernetes.io/os: linux
  topology.kubernetes.io/zone: fi-hel1
matchExpressions:
  - { key: node-role.kubernetes.io/control-plane, operator: DoesNotExist}
  - { key: node-role.kubernetes.io/master, operator: DoesNotExist}
```
If left empty, then nodes filtered only by CCM node scope selector

4. Auto TLS provision
You can force automatic TLS certificate provision for load balancer (1 auto TLS per load balancer).
The auto provisioned TLS certificate can be bound to any frontend.
This feature can be of help if you want to expose HTTPS endpoint but you don't have prepared upfront TLS certificates.
In order to fulfill it, the following API is used: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#create-dynamic-certificate-bundle
(the certificate key type is `ecdsa`).

The auto TLS provision is supported only for frontends with `mode` set to `http`.
To enable this feature, you need to provide specific frontend config in `service.beta.kubernetes.io/upcloud-loadbalancer-config`:
```json
 "frontends": [
        {
            "name": "must match port name in spec.ports",
            "mode": "http", // 'http' 
            "port": 8443, // Any arbitrary port
         
            // Description: https://developers.upcloud.com/1.3/17-managed-loadbalancer/#create-tls-config
            "tls_configs": [
                {
                    "name": "needs-certificate" // special value that triggers logic for auto TLS provision
                }
            ],
        }
    ]
```
Special case for `443` port:
If the frontend `mode` set to `http` and port set to `443` & `tls_config` is not set, then tls_config  for this forntend
will be automatically set to:
```json
"tls_configs": [
  {
    "name": "needs-certificate"
  }
]
```
that triggers the auto TLS.

4. Automatically created annotations that support load-balancer provision integrity. Do not delete or modify them.
The following annotations created during load-balancer provision:
- `service.beta.kubernetes.io/upcloud-load-balancer-id` - this annotation specify the load-balancer ID
  used to enable fast retrievals of load-balancers from the API by UUID.
- `service.beta.kubernetes.io/upcloud-load-balancer-name` - is the annotation reporting the UpCloud load-balancer name as seen in the hub.upcloud.com
