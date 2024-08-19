package controllers

import (
	"context"
	"fmt"
	ciliumhaegress "github.com/angeloxx/cilium-ha-egress/pkg"
	"github.com/cilium/cilium/pkg/hubble/relay/defaults"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServicesController struct {
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

func (r *ServicesController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var service = corev1.Service{}
	var log = r.Log

	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch the Service, check RBAC permissions")
		return ctrl.Result{}, err
	}

	logger := log.WithValues("namespace", service.Namespace, "service", service.Name)

	// Ignores services without labels managed by us
	if service.Labels[ciliumhaegress.HAEgressIPName] == "" || service.Labels[ciliumhaegress.HAEgressIPNamespace] == "" {
		return ctrl.Result{}, nil
	}

	// Update CiliumEgressGatewayPolicy with the LoadBalancerIP
	ciliumEgressGatewayPolicy := &ciliumv2.CiliumEgressGatewayPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-%s-%s",
		ciliumhaegress.CiliumEgressGatewayPolicyNamePrefix,
		service.Labels[ciliumhaegress.HAEgressIPNamespace], service.Labels[ciliumhaegress.HAEgressIPName])}, ciliumEgressGatewayPolicy)

	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("CiliumEgressGatewayPolicy not found, we probably are waiting for automatic creation")
			return ctrl.Result{RequeueAfter: defaults.HealthCheckInterval}, err
		} else {
			logger.Error(err, "unable to fetch the CiliumEgressGatewayPolicy, review RBAC permissions")
			return ctrl.Result{}, err
		}
	}

	if len(service.Status.LoadBalancer.Ingress) > 0 {
		if ciliumEgressGatewayPolicy.Spec.EgressGateway.EgressIP != service.Status.LoadBalancer.Ingress[0].IP {

			ciliumEgressGatewayPolicy.Spec.EgressGateway.EgressIP = service.Status.LoadBalancer.Ingress[0].IP
			if err := r.Update(ctx, ciliumEgressGatewayPolicy); err != nil {
				logger.Error(err, "unable to update the CiliumEgressGatewayPolicy")
				return ctrl.Result{}, err
			}
			logger.Info("Updated CiliumEgressGatewayPolicy with LoadBalancerIP", "LoadBalancerIP", service.Status.LoadBalancer.Ingress[0].IP)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServicesController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
