![Logo](docs/img/kube-vip-cilium-watcher_mini.png)

# kube-vip-cilium-watcher
This operator is used in an environment where you want to use Cilium as Ingress and Egress traffic manager. 

## Description
Due the limitation of CiliumEgressGatewayPolicy, it is not possible to implement freely an HA solution where the policy defines
two egress IP or the IP is moved automatically from a node to another.
You can use kube-vip to create a virtual IP that is moved from a node to another in case of failure. When kube-vip
associate a service to a node, it annotates associated service with kube-vip.io/vipHost: <node-name>. This operator
watches for this annotation and updates the CiliumEgressPolicy to select the node where the service is running and
implement a floating egress ip.

## Installation

Helm chart will be provided.

## Configure

Configure the service as a virtual ip managed by kuve-vip. The service must be of type LoadBalancer and set

    spec.loadBalancerClass: "kube-vip.io/kube-vip-class"

in order to let kube-vip manage the service. Additionally the annotation:

    kube-vip.io/cilium-egress-watcher: "true"

has to be added to the service. You have to add to all nodes that runs kube-vip the label:

    kube-vip.io/host: "<host-shortname>"

The CiliumEgressGatewayPolicy(es) that matches the service loadBalancerIps with spec.egressGateway.egressIP will
be reconfigured with a spec.egressGateway.nodeSelector that matches the "kube-vip.io/vipHost" label in order to 
route the traffic to that node.

## License

Copyright 2024 Angelo Conforti.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

