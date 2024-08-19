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

package controllers

import (
	"context"
	"fmt"
	haegressip "github.com/angeloxx/cilium-ha-egress/api/v1alpha1"
	ciliumhaegress "github.com/angeloxx/cilium-ha-egress/pkg"
	"github.com/cilium/cilium/pkg/hubble/relay/defaults"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CiliumEgressGatewayPolicyReconciler reconciles a CiliumEgressGatewayPolicy object
type CiliumEgressGatewayPolicyReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	CiliumNamespace string
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumegressgatewaypolicies,verbs=get;update;patch;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CiliumEgressGatewayPolicy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *CiliumEgressGatewayPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var egressPolicy ciliumv2.CiliumEgressGatewayPolicy
	var log = r.Log

	if err := r.Get(ctx, req.NamespacedName, &egressPolicy); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch CiliumEgressGatewayPolicy")
		return ctrl.Result{}, err
	}
	logger := log.WithValues("egressgatewaypolicy", req.Name)

	if egressPolicy.Labels[ciliumhaegress.HAEgressIPNamespace] == "" || egressPolicy.Labels[ciliumhaegress.HAEgressIPName] == "" {
		logger.V(1).Info("EgressGatewayPolicy doesn't have the lease annotation, ignoring")
		return ctrl.Result{}, nil
	}

	haegressipNamespace := egressPolicy.Labels[ciliumhaegress.HAEgressIPNamespace]
	haegressipName := egressPolicy.Labels[ciliumhaegress.HAEgressIPName]

	// If HAEgressIP resource is gone, we can remove this resource (because we can't link it to a namespaced object)
	haEgressIP := haegressip.HAEgressIP{}
	if err := r.Get(ctx, types.NamespacedName{Name: haegressipName, Namespace: haegressipNamespace}, &haEgressIP); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("HAEgressIP resource not found, removing egress gateway policy")
			if err := r.Delete(ctx, &egressPolicy); err != nil {
				logger.Error(err, "Unable to delete egress gateway policy")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to get HAEgressIP resource")
		return ctrl.Result{}, err
	}

	leaseFullName := fmt.Sprintf("cilium-l2announce-%s-%s-%s", haegressipNamespace, ciliumhaegress.ServiceNamePrefix, haegressipName)

	// Get the lease
	var lease v1.Lease
	if err := r.Get(ctx, types.NamespacedName{Name: leaseFullName, Namespace: r.CiliumNamespace}, &lease); err != nil {
		// Debug log
		logger.Info(fmt.Sprintf("Lease %s/%s not found, retry later in %s", r.CiliumNamespace, leaseFullName, defaults.HealthCheckInterval))
		return ctrl.Result{RequeueAfter: defaults.HealthCheckInterval}, nil
	}

	host := *lease.Spec.HolderIdentity
	currentHost := egressPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[ciliumhaegress.NodeNameAnnotation]

	if currentHost == "" || currentHost != host {
		// Modify egressPolicy nodeSepector to match the service
		patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, ciliumhaegress.NodeNameAnnotation, host)

		logger.Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, host))
		if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
			logger.Error(err, fmt.Sprintf("Unable to patch cilium egress gateway policy %s", egressPolicy.Name))
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&egressPolicy, "Normal", ciliumhaegress.EventEgressUpdateReason, fmt.Sprintf("Updated with new nodeSelector %s=%s by %s/%s service", ciliumhaegress.NodeNameAnnotation, host, haegressipNamespace, haegressipName))
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CiliumEgressGatewayPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ciliumv2.CiliumEgressGatewayPolicy{}).
		Complete(r)
}
