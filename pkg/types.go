package haegressip

import "time"

const (
	HAEgressGatewayPolicyNamespace         = "cilium.angeloxx.ch/haegressgatewaypolicy-namespace"
	HAEgressGatewayPolicyName              = "cilium.angeloxx.ch/haegressgatewaypolicy-name"
	HAEgressGatewayPolicyExpectedLeaseName = "cilium.angeloxx.ch/lease-name"
	NodeNameAnnotation                     = "kubernetes.io/hostname"
	EventEgressUpdateReason                = "Updated"
	ServiceNamePrefix                      = "haegress"
	CiliumEgressGatewayPolicyNamePrefix    = "haegress"

	LeaseCheckRequeueAfter                 = 10 * time.Second
	HAEgressGatewayPolicyChcekRequeueAfter = 10 * time.Second
)
