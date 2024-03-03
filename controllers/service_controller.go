package controllers

import (
	"context"
	"fmt"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	serviceMustBeWatched = "kube-vip.io/cilium-egress-watcher"
	kubeVipAnnotation    = "kube-vip.io/vipHost"
)

// Reconcile handles a reconciliation request for a Service with the
// kube-vip-cilium-watcher annotation.
// If the annotation is absent, then Reconcile will ignore the service.

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumegressgatewaypolicies,verbs=get;update;patch

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var service corev1.Service
	var ips []string

	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Service")
		return ctrl.Result{}, err
	}
	logger := log.WithFields(
		log.Fields{
			"namespace": req.Namespace,
			"service":   req.Name,
		})

	serviceShouldBeChecked := service.Annotations[serviceMustBeWatched] == "true"
	if !serviceShouldBeChecked {
		logger.Debug("Service does not have the annotation, ignoring")
		return ctrl.Result{}, nil
	}

	serviceHasHostAssociated := service.Annotations[kubeVipAnnotation] != ""
	if !serviceHasHostAssociated {
		logger.Debug("service doesn't have a host associated, ignoring")
		return ctrl.Result{}, nil
	}
	host := service.Annotations[kubeVipAnnotation]

	// Check if the service has a loadBalancerIP or loadBalancerIPs
	if service.Status.LoadBalancer.Ingress == nil {
		logger.Debug("service doesn't have an assigned IP address, ignoring")
		return ctrl.Result{}, nil
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		ips = append(ips, ingress.IP)
	}

	if len(ips) == 0 {
		logger.Debug("service has the annotation but no loadBalancerIP(s), ignoring")
		return ctrl.Result{}, nil
	}

	logger.Infof("has the annotation, ips are %s, checking if a cilium egress must be modified",
		strings.Join(ips[:], ","))

	// get all cilium egress gateway policies from api server
	var egressPolicies ciliumv2.CiliumEgressGatewayPolicyList
	if err := r.List(ctx, &egressPolicies); err != nil {
		logger.Error(err, "unable to list cilium egress gateway policies")
		return ctrl.Result{}, err
	}
	logger.Infof("Found %d Cilium egress gateway policies to evaluate", len(egressPolicies.Items))
	for _, egressPolicy := range egressPolicies.Items {
		if slices.Contains(ips, egressPolicy.Spec.EgressGateway.EgressIP) {
			// Modify egressPolicy nodeSepector to match the service
			patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"kube-vip.io/host":"%s"}}}}}`, host)

			logger.Infof("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, host)
			if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
				logger.Errorf("unable to patch cilium egress gateway policy %s", egressPolicy.Name)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
