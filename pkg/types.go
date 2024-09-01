package haegressip

import "time"

const (
	HAEgressGatewayPolicyNamespace       = "cilium.angeloxx.ch/haegressgatewaypolicy-namespace"
	HAEgressGatewayPolicyName            = "cilium.angeloxx.ch/haegressgatewaypolicy-name"
	NodeNameAnnotation                   = "kubernetes.io/hostname"
	EventEgressUpdateReason              = "Updated"
	KubeVIPVipHostAnnotation             = "kube-vip.io/vipHost"
	KubernetesServiceProxyNameAnnotation = "service.kubernetes.io/service-proxy-name"

	LeaseCheckRequeueAfter                 = 10 * time.Second
	HAEgressGatewayPolicyChcekRequeueAfter = 10 * time.Second
)
