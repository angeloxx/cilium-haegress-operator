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
	"github.com/angeloxx/kube-vip-cilium-watcher/pkg"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CiliumEgressGatewayPolicyReconciler reconciles a CiliumEgressGatewayPolicy object
type CiliumEgressGatewayPolicyReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
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
	var ips []string
	var log = r.Log

	if err := r.Get(ctx, req.NamespacedName, &egressPolicy); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Service")
		return ctrl.Result{}, err
	}
	logger := log.WithValues("egressgatewaypolicy", req.Name)

	// When a new policy is configured and matches a kube-vip service we have to patch it
	// 	patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"kube-vip.io/host":"%s"}}}}}`, host)

	logger.Info(fmt.Sprintf("EgressGatewayPolicy has IP %s, checking for services", egressPolicy.Spec.EgressGateway.EgressIP))

	// Get the list of all matching services and determine current kube-vio.io/vipHost that runs that IP
	var services = corev1.ServiceList{}
	if err := r.List(ctx, &services); err != nil {
		logger.Error(err, "Unable to list services, check RBAC permissions")
		return ctrl.Result{}, err
	}

	logger.Info(fmt.Sprintf("Found %d Services to evaluate", len(services.Items)))
	for _, service := range services.Items {
		serviceLogger := logger.WithValues("service", service.Name, "namespace", service.Namespace)

		serviceShouldBeChecked := service.Annotations[kubevipciliumwatcher.ServiceMustBeWatched] == "true"
		if !serviceShouldBeChecked {
			serviceLogger.V(1).Info("Service does not have the annotation, ignoring")
			continue
		}

		serviceHasHostAssociated := service.Annotations[kubevipciliumwatcher.KubeVipAnnotation] != ""
		if !serviceHasHostAssociated {
			serviceLogger.V(1).Info("service doesn't have a host associated, ignoring")
			continue
		}
		host := service.Annotations[kubevipciliumwatcher.KubeVipAnnotation]

		for _, ingress := range service.Status.LoadBalancer.Ingress {
			ips = append(ips, ingress.IP)
		}

		if len(ips) == 0 {
			serviceLogger.V(1).Info("Service has the annotation but no loadBalancerIP(s), ignoring")
			continue
		}

		if slices.Contains(ips, egressPolicy.Spec.EgressGateway.EgressIP) {
			if egressPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[kubevipciliumwatcher.EgressVipAnnotation] == host {
				logger.Info("EgressGatewayPolicy already configured as expected, ignoring.")
				return ctrl.Result{}, nil
			}

			// Modify egressPolicy nodeSepector to match the service
			patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, kubevipciliumwatcher.EgressVipAnnotation, host)

			serviceLogger.Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, host))
			if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
				serviceLogger.Error(err, fmt.Sprintf("Unable to patch cilium egress gateway policy %s", egressPolicy.Name))
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CiliumEgressGatewayPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ciliumv2.CiliumEgressGatewayPolicy{}).
		Complete(r)
}
