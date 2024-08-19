package ciliumhaegress

const (
	HAEgressIPNamespace                 = "cilium.angeloxx.ch/haegressip-namespace"
	HAEgressIPName                      = "cilium.angeloxx.ch/haegressip-name"
	HAEgressIPExpectedLeaseName         = "cilium.angeloxx.ch/lease-name"
	NodeNameAnnotation                  = "kubernetes.io/hostname"
	EventEgressUpdateReason             = "Updated"
	ServiceNamePrefix                   = "egress"
	CiliumEgressGatewayPolicyNamePrefix = "egress"
)
