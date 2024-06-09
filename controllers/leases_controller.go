package controllers

import (
	"context"
	"fmt"
	kubevipciliumwatcher "github.com/angeloxx/kube-vip-cilium-watcher/pkg"
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
// kube-vip-cilium-watcher annotation.
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
	// get all cilium egress gateway policies from api server
	var egressPolicies ciliumv2.CiliumEgressGatewayPolicyList
	if err := r.List(ctx, &egressPolicies); err != nil {
		logger.Error(err, "unable to list cilium egress gateway policies, check RBAC permissions")
		return ctrl.Result{}, err
	}

	logger.V(0).Info(fmt.Sprintf("Found %d Cilium egress gateway policies to evaluate", len(egressPolicies.Items)))
	for _, egressPolicy := range egressPolicies.Items {
		leaseFullName := fmt.Sprintf("cilium-l2announce-%s-%s", egressPolicy.Annotations[kubevipciliumwatcher.LeaseServiceNamespace], egressPolicy.Annotations[kubevipciliumwatcher.LeaseServiceName])
		if leaseFullName != lease.Name {
			continue
		}

		currentHost := *lease.Spec.HolderIdentity
		if currentHost == "" {
			logger.Info("Lease doesn't have a holderIdentity, ignoring")
			continue
		}

		policyHost := string(egressPolicy.Spec.EgressGateway.NodeSelector.MatchLabels[kubevipciliumwatcher.NodeNameAnnotation])

		if policyHost == currentHost {
			logger.Info("EgressGatewayPolicy already configured as expected, ignoring.")
			continue
		}

		// Modify egressPolicy nodeSepector to match the service
		patchData := fmt.Sprintf(`{"spec":{"egressGateway":{"nodeSelector":{"matchLabels":{"%s":"%s"}}}}}`, kubevipciliumwatcher.NodeNameAnnotation, currentHost)

		logger.V(0).Info(fmt.Sprintf("Patching cilium egress gateway policy %s with host %s", egressPolicy.Name, currentHost))
		if err := r.Patch(ctx, &egressPolicy, client.RawPatch(types.MergePatchType, []byte(patchData))); err != nil {
			logger.V(0).Info("unable to patch cilium egress gateway policy %s", egressPolicy.Name)
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&egressPolicy, "Normal", kubevipciliumwatcher.EventEgressUpdateReason, fmt.Sprintf("Updated with new nodeSelector %s=%s by %s/%s service", kubevipciliumwatcher.NodeNameAnnotation, currentHost, req.Namespace, req.Name))

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeasesController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Lease{}).
		Complete(r)
}
