package kubevipciliumwatcher

const (
	ServiceMustBeWatched       = "kube-vip.io/cilium-egress-watcher"
	KubeVipAnnotation          = "kube-vip.io/vipHost"
	EgressVipAnnotation        = "kube-vip.io/host"
	EventServiceUpdateReason   = "EgressAssigned"
	EventServiceNotFoundReason = "EgressNotFound"
	EventEgressUpdateReason    = "Updated"
)
