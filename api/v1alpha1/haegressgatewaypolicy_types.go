/*
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
*/

// ref https://github.com/cilium/cilium/blob/main/pkg/k8s/apis/cilium.io/v2/cegp_types.go
package v1alpha1

import (
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HAEgressGatewayPolicy defines the observed state of haEgressGatewayPolicy
type HAEgressGatewayPolicyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ServiceCreated bool `json:"serviceCreated"`
	PolicyCreated  bool `json:"policyCreated"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// haEgressGatewayPolicy is the Schema for the haegressgatewaypolicies API
type HAEgressGatewayPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ciliumv2.CiliumEgressGatewayPolicySpec `json:"spec,omitempty"`
	Status HAEgressGatewayPolicyStatus            `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// haEgressGatewayPolicyList contains a list of haEgressGatewayPolicy
type HAEgressGatewayPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HAEgressGatewayPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HAEgressGatewayPolicy{}, &HAEgressGatewayPolicyList{})
}