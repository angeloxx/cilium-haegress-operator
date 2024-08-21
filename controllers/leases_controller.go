package controllers

import (
	"context"
	"fmt"
	haegressip "github.com/angeloxx/cilium-haegress-operator/pkg"
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
	EgressNamespace string
}

// Reconcile handles a reconciliation request for a Lease with the
// cilium-haegress-operator annotation.
// If the annotation is absent, then Reconcile will ignore the service.

// +kubebuilder:rbac:groups=core,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumegressgatewaypolicies,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *LeasesController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// ignore useless leases
	if !strings.HasPrefix(req.Name, "cilium-l2announce-") {
		return ctrl.Result{}, nil
	}

	logger := r.Log.WithValues("namespace", req.Namespace, "lease", req.Name)

	var lease v1.Lease
	if err := r.Get(ctx, req.NamespacedName, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch the Lease, check RBAC permissions")
		return ctrl.Result{}, err
	}

	currentHost := *lease.Spec.HolderIdentity
	if currentHost == "" {
		logger.Info("Lease doesn't have a holderIdentity, ignoring and reconcile on next update")
		return ctrl.Result{}, nil
	}

	// check the lease was already annotated with the service name generated the lease
	/*	isLeaseAnnotated := lease.Labels[haegressip.HAEgressGatewayPolicyName] != "" && lease.Labels[haegressip.HAEgressGatewayPolicyNamespace] != ""
		if !isLeaseAnnotated {
			logger.V(1).Info("lease doesn't have cilium-haegress-operator annotations, we'll be updated in this reconcile if found")

			// Search for the HAEgressGatewayPolicy that matches the lease
			var haEgressGatewayPolicyList v1alpha1.HAEgressGatewayPolicyList
			lookupHAEgressGatewayPolicyResult := r.List(ctx, &haEgressGatewayPolicyList, client.MatchingLabels{
				haegressip.HAEgressGatewayPolicyExpectedLeaseName: lease.Name})
			if lookupHAEgressGatewayPolicyResult != nil {
				logger.Error(lookupHAEgressGatewayPolicyResult, "unable to list HAEgressGatewayPolicy, check RBAC permissions or CRD not installed")
				return ctrl.Result{RequeueAfter: haegressip.LeaseCheckRequeueAfter}, lookupHAEgressGatewayPolicyResult
			}

			logger.V(1).Info(fmt.Sprintf("Found %d haegressgatewaypolicies to evaluate", len(haEgressGatewayPolicyList.Items)))
			for _, egressHAIP := range haEgressGatewayPolicyList.Items {
				logger.Info(fmt.Sprintf("Updating lease %s with reference to parent HAEgressGatewayPolicy", lease.Name))
				patchData := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s","%s":"%s"}}}`,
					haegressip.HAEgressGatewayPolicyName, egressHAIP.Name,
					haegressip.HAEgressGatewayPolicyNamespace, r.EgressNamespace)
				if err := r.Patch(ctx, &lease, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
					logger.Error(err, "unable to update lease with cilium-haegress-operator annotations")
					return ctrl.Result{}, err
				}
				logger.V(0).Info(fmt.Sprintf("Lease %s updated with cilium-haegress-operator annotations", lease.Name))
				isLeaseAnnotated = true
				break
			}
		}

		if !isLeaseAnnotated {
			logger.V(1).Info("lease doesn't have cilium-haegress-operator annotations, ignoring")
			return ctrl.Result{}, nil

		}*/

	var egressPolicies ciliumv2.CiliumEgressGatewayPolicyList
	if err := r.List(ctx, &egressPolicies, client.MatchingLabels{
		haegressip.HAEgressGatewayPolicyExpectedLeaseName: lease.Name,
	}); err != nil {
		logger.Error(err, "unable to list HAEgressGatewayPolicy, check RBAC permissions or CRD not installed")
		return ctrl.Result{}, err
	}

	if len(egressPolicies.Items) == 0 {
		logger.V(1).Info(fmt.Sprintf("No CiliumEgressGatewayPolicy found for the lease, ignoring"))
		return ctrl.Result{}, nil
	}
	logger.V(1).Info(fmt.Sprintf("Found %d policies to evaluate (filtered)", len(egressPolicies.Items)))
	for _, egressPolicy := range egressPolicies.Items {
		policyHost := string(egressPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[haegressip.NodeNameAnnotation])
		if policyHost == currentHost {
			logger.V(1).Info("EgressGatewayPolicy already configured as expected, ignoring.")
			continue
		}
		logger.V(0).Info(fmt.Sprintf("EgressGatewayPolicy should be updated from %s to %s.", policyHost, currentHost))

		// Modify egressPolicy nodeSelector to match the service
		patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, haegressip.NodeNameAnnotation, currentHost)

		logger.V(0).Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, currentHost))
		if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
			logger.V(0).Info(fmt.Sprintf("unable to patch cilium egress gateway policy %s", egressPolicy.Name))
			return ctrl.Result{RequeueAfter: haegressip.LeaseCheckRequeueAfter}, err
		}
		r.Recorder.Event(&egressPolicy, "Normal", haegressip.EventEgressUpdateReason, fmt.Sprintf("Updated with new nodeSelector %s=%s by %s/%s service", haegressip.NodeNameAnnotation, currentHost, req.Namespace, req.Name))

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeasesController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Lease{}).
		Complete(r)
}
