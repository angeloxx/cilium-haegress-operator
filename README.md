![Logo](https://github.com/angeloxx/cilium-haegress-operator/raw/main/docs/img/cilium-haegress-operator_mini.png)

# cilium-haegress-operator
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

This operator is used in an environment where you want to use Cilium as Ingress and Egress traffic manager. 

## Description
Due the limitation of CiliumEgressGatewayPolicy, it is not possible to implement freely an HA solution where the policy 
defines two egress IP or the IP is moved automatically from a node to another.
You can use this project to create a virtual IP that is moved from a node to another in case of failure. When kube-vip
associate a service to a node, it annotates associated service with kube-vip.io/vipHost: <node-name>. This operator
watches for this annotation and updates the CiliumEgressPolicy to select the node where the service is running and
implement a floating egress ip.

## Installation

You can use Helm and the default settings to install the operator:

```shell
helm upgrade -i cilium-haegress-operator --create-namespace --namespace egress-management
     oci://registry-1.docker.io/angeloxx/cilium-haegress-operator --version x.x.x-helm
```

## Configure

You can configure a new HAEgressGatewayPolicy using the following yaml:

```yaml
apiVersion: cilium.angeloxx.ch/v2
kind: HAEgressGatewayPolicy
metadata:
  annotations:
    kube-vip.io/loadbalancerIPs: 192.168.152.10
  name: egress-192-168-152-10
spec:
  destinationCIDRs:
    - 0.0.0.0/0
  egressGateway:
    nodeSelector:
      matchLabels:
        your.company/egress-node: "true"
  selectors:
    - podSelector:
        matchLabels:
          io.kubernetes.pod.namespace: my-beautiful-namespace
```
Using the 

    kube-vip.io/loadbalancerIPs

annotation kube-vip will assign that IP but you can also omit the annotation and kube-vip will assign an IP from the
configured pool. The operator will create:

* a CiliumEgressGatewayPolicy named <service-namespace>-<haegressgatewaypolicy-name>
* a Service managed by Kube-VIP, with the same name in the operator namespace 

if you want to change the service namespace, you can use the annotation:

    cilium.angeloxx.ch/haegressgatewaypolicy-namespace: the-egress-namespace

and the service will be created in that namespace.

The Operator will link the service and the CiliumEgressGatewayPolicy; when the IP address is assigned, it will be configured as EgressIP and
when the services is assigned to a specific node, the CiliumEgressGatewayPolicy nodeSelector will be updated. 

All these three objects will be linked: if the HAEgressGatewayPolicy is deleted, the service and the CiliumEgressGatewayPolicy will be deleted too.
If the policy or the service is accidentally deleted, the operator will recreate and synchronize them.

## # Kubectl

You can check the status of the HAEgressIPs status using kubectl:

```shell
user@host:> kubectl get Haegressgatewaypolicies
NAME                     IP ADDRESS       EXIT NODE                      AGE
egress-192-168-152-10    192.168.152.10   egress-node-004.domain.local   77m
egress-192-168-152-11    192.168.152.11   egress-node-003.domain.local   76m
egress-192-168-152-12    192.168.152.12   egress-node-004.domain.local   76m
egress-192-168-152-13    192.168.152.13   egress-node-004.domain.local   77m
egress-192-168-152-15    192.168.152.15   egress-node-004.domain.local   76m
egress-192-168-152-18    192.168.152.18   egress-node-004.domain.local   76m
egress-192-168-152-19    192.168.152.19   egress-node-004.domain.local   77m
```
The status will report the name of the resource, the assigned IP by kube-vip, the node where the IP is assigned and when the last change has occurred.

## License

    Copyright (C) 2024 Angelo Conforti.

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU Affero General Public License as
    published by the Free Software Foundation, either version 3 of the
    License, or (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU Affero General Public License for more details.

    You should have received a copy of the GNU Affero General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.

