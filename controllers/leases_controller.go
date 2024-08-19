package controllers

import (
	"context"
	"fmt"
	haegressip "github.com/angeloxx/cilium-ha-egress/api/v1alpha1"
	ciliumhaegress "github.com/angeloxx/cilium-ha-egress/pkg"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type LeasesController struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	CiliumNamespace string
}

// Reconcile handles a reconciliation request for a Lease with the
// cilium-ha-egress annotation.
// If the annotation is absent, then Reconcile will ignore the service.

// +kubebuilder:rbac:groups=core,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumegressgatewaypolicies,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *LeasesController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var lease v1.Lease
	var log = r.Log

	if err := r.Get(ctx, req.NamespacedName, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch the Lease")
		return ctrl.Result{}, err
	}

	// ignore useless leases
	if !strings.HasPrefix(lease.Name, "cilium-l2announce-") {
		return ctrl.Result{}, nil
	}

	logger := log.WithValues("namespace", req.Namespace, "lease", req.Name)

	currentHost := *lease.Spec.HolderIdentity
	if currentHost == "" {
		logger.Info("Lease doesn't have a holderIdentity, ignoring and reconcile on update")
		return ctrl.Result{}, nil
	}

	// check the lease was already annotated with the service name generated the lease
	isLeaseAnnotated := lease.Labels[ciliumhaegress.HAEgressIPName] != "" && lease.Labels[ciliumhaegress.HAEgressIPNamespace] != ""
	if !isLeaseAnnotated {
		logger.V(1).Info("lease doesn't have cilium-ha-egress annotations, we'll be updated in this reconcile if found")

		// Get the HAEgressIP that matches this lease
		var egressHAIPs haegressip.HAEgressIPList
		if err := r.List(ctx, &egressHAIPs); err != nil {
			logger.Error(err, "unable to list HA Cilium Egress IPs, check RBAC permissions")
			return ctrl.Result{}, err
		}
		// Search for the HAEgressIP that matches the lease
		lookupMatchingEgressHAIPs := r.List(ctx, &egressHAIPs, client.MatchingLabels{
			ciliumhaegress.HAEgressIPExpectedLeaseName: lease.Name})
		if lookupMatchingEgressHAIPs != nil {
			logger.Error(lookupMatchingEgressHAIPs, "unable to list HA Cilium Egress IPs, check RBAC permissions")
			return ctrl.Result{}, lookupMatchingEgressHAIPs
		}

		logger.V(1).Info(fmt.Sprintf("Found %d HAEgressIPs to evaluate", len(egressHAIPs.Items)))
		for _, egressHAIP := range egressHAIPs.Items {
			if fmt.Sprintf("cilium-l2announce-%s-%s-%s", egressHAIP.Namespace, ciliumhaegress.ServiceNamePrefix, egressHAIP.Name) == lease.Name {
				logger.Info(fmt.Sprintf("Updating lease %s with reference to parent HAEgressIP", lease.Name))
				patchData := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s","%s":"%s"}}}`,
					ciliumhaegress.HAEgressIPName, egressHAIP.Name,
					ciliumhaegress.HAEgressIPNamespace, egressHAIP.Namespace)
				if err := r.Patch(ctx, &lease, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
					logger.Error(err, "unable to update lease with cilium-ha-egress annotations")
					return ctrl.Result{}, err
				}
				logger.V(0).Info(fmt.Sprintf("Lease %s updated with cilium-ha-egress annotations", lease.Name))
				isLeaseAnnotated = true
				break
			}
		}
	}

	if !isLeaseAnnotated {
		logger.V(1).Info("lease doesn't have cilium-ha-egress annotations, ignoring")
		return ctrl.Result{}, nil

	}

	var egressPolicies ciliumv2.CiliumEgressGatewayPolicyList
	if err := r.List(ctx, &egressPolicies, client.MatchingLabels{
		ciliumhaegress.HAEgressIPNamespace: lease.Labels[ciliumhaegress.HAEgressIPNamespace],
		ciliumhaegress.HAEgressIPName:      lease.Labels[ciliumhaegress.HAEgressIPName],
	}); err != nil {
		logger.Error(err, "unable to list cilium egress gateway policies, check RBAC permissions")
		return ctrl.Result{}, err
	}

	if len(egressPolicies.Items) == 0 {
		logger.V(1).Info(fmt.Sprintf("No CiliumEgressGatewayPolicy found for the lease, ignoring"))
		return ctrl.Result{}, nil
	}
	logger.V(1).Info(fmt.Sprintf("Found %d policies to evaluate (filtered)", len(egressPolicies.Items)))

	for _, egressPolicy := range egressPolicies.Items {

		policyHost := string(egressPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[ciliumhaegress.NodeNameAnnotation])

		if policyHost == currentHost {
			logger.V(1).Info("EgressGatewayPolicy already configured as expected, ignoring.")
			continue
		}

		logger.V(0).Info(fmt.Sprintf("EgressGatewayPolicy should be updated from %s to %s.", policyHost, currentHost))

		// Modify egressPolicy nodeSelector to match the service
		patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, ciliumhaegress.NodeNameAnnotation, currentHost)

		logger.V(0).Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, currentHost))
		if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
			logger.V(0).Info("unable to patch cilium egress gateway policy %s", egressPolicy.Name)
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&egressPolicy, "Normal", ciliumhaegress.EventEgressUpdateReason, fmt.Sprintf("Updated with new nodeSelector %s=%s by %s/%s service", ciliumhaegress.NodeNameAnnotation, currentHost, req.Namespace, req.Name))

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeasesController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Lease{}).
		Complete(r)
}
