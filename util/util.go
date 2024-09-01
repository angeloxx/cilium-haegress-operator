package util

import (
	"context"
	"fmt"
	v2 "github.com/angeloxx/cilium-haegress-operator/api/v2"
	haegressip "github.com/angeloxx/cilium-haegress-operator/pkg"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SyncServiceWithCiliumEgressGatewayPolicy(ctx context.Context, r client.Client, logger logr.Logger, recorder record.EventRecorder, service corev1.Service, ciliumEgressGatewayPolicy ciliumv2.CiliumEgressGatewayPolicy) (ctrl.Result, error) {

	// Get the parent HAEgressGatewayPolicy from the ciliumEgressGatewayPolicy
	haEgressGatewayPolicy := &v2.HAEgressGatewayPolicy{}
	ownerRefs := ciliumEgressGatewayPolicy.GetOwnerReferences()
	for _, ownerRef := range ownerRefs {
		if ownerRef.Kind == "HAEgressGatewayPolicy" {
			if err := r.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: ciliumEgressGatewayPolicy.Namespace}, haEgressGatewayPolicy); err != nil {
				logger.Error(err, "unable to fetch the HAEgressGatewayPolicy, check RBAC permissions")
				return ctrl.Result{}, nil
			}
			break
		}
	}

	policyHost := string(ciliumEgressGatewayPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[haegressip.NodeNameAnnotation])
	currentHost := string(service.Annotations[haegressip.KubeVIPVipHostAnnotation])

	if len(service.Status.LoadBalancer.Ingress) > 0 {
		if ciliumEgressGatewayPolicy.Spec.EgressGateway.EgressIP != service.Status.LoadBalancer.Ingress[0].IP {
			ciliumEgressGatewayPolicy.Spec.EgressGateway.EgressIP = service.Status.LoadBalancer.Ingress[0].IP
			if err := r.Update(ctx, &ciliumEgressGatewayPolicy); err != nil {
				logger.Error(err, "unable to update the CiliumEgressGatewayPolicy with new assigned IP, retry later")
				return ctrl.Result{RequeueAfter: haegressip.HAEgressGatewayPolicyChcekRequeueAfter}, nil
			}
			logger.Info("Updated CiliumEgressGatewayPolicy with LoadBalancerIP", "LoadBalancerIP", service.Status.LoadBalancer.Ingress[0].IP)

		}
		if haEgressGatewayPolicy.Status.IPAddress != service.Status.LoadBalancer.Ingress[0].IP {
			haEgressGatewayPolicy.Status.IPAddress = service.Status.LoadBalancer.Ingress[0].IP
			if err := r.Status().Update(ctx, haEgressGatewayPolicy); err != nil {
				logger.Error(err, "unable to update the HAEgressGatewayPolicy with new assigned IP")
			}
		}
	}

	if currentHost == "" {
		logger.V(1).Info(fmt.Sprintf("Service is still not assigned, ignoring."))
		return ctrl.Result{}, nil
	}

	if haEgressGatewayPolicy.Status.ExitNode != currentHost {
		haEgressGatewayPolicy.Status.ExitNode = currentHost
		if err := r.Status().Update(ctx, haEgressGatewayPolicy); err != nil {
			logger.Error(err, "unable to update the HAEgressGatewayPolicy with new assigned exitNode")
		}
	}

	if policyHost == currentHost {
		logger.V(1).Info(fmt.Sprintf("EgressGatewayPolicy already configured as expected with host %s, ignoring.", currentHost))
		return ctrl.Result{}, nil
	}

	logger.V(0).Info(fmt.Sprintf("EgressGatewayPolicy should be updated from %s to %s.", policyHost, currentHost))

	// Modify egressPolicy nodeSelector to match the service
	patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, haegressip.NodeNameAnnotation, currentHost)

	logger.V(0).Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", ciliumEgressGatewayPolicy.Name, currentHost))
	if err := r.Patch(ctx, &ciliumEgressGatewayPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
		logger.V(0).Info(fmt.Sprintf("Unable to patch cilium egress gateway policy %s", ciliumEgressGatewayPolicy.Name))
		return ctrl.Result{RequeueAfter: haegressip.LeaseCheckRequeueAfter}, err
	}

	recorder.Event(&ciliumEgressGatewayPolicy, "Normal",
		haegressip.EventEgressUpdateReason,
		fmt.Sprintf("Updated with new nodeSelector %s=%s by %s/%s service",
			haegressip.NodeNameAnnotation, currentHost,
			service.Namespace, service.Name))

	recorder.Event(&service, "Normal",
		haegressip.EventEgressUpdateReason,
		fmt.Sprintf("Updated CiliumEgressGatewayPolicy %s with new nodeSelector %s=%s",
			ciliumEgressGatewayPolicy.Name,
			haegressip.NodeNameAnnotation, currentHost))
	return ctrl.Result{}, nil
}
