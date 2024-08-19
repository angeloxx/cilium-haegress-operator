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

package main

import (
	"flag"
	"os"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	//log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ciliumangeloxxchv1alpha1 "github.com/angeloxx/cilium-ha-egress/api/v1alpha1"
	"github.com/angeloxx/cilium-ha-egress/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	err := ciliumv2.AddToScheme(scheme)
	if err != nil {
		return
	}

	utilruntime.Must(ciliumangeloxxchv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var ciliumNamespace string
	var k8sClientQPS int
	var k8sClientBurst int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&ciliumNamespace, "cilium-namespace", "kube-system", "The namespace where Cilium is installed")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&k8sClientQPS, "k8s-client-qps", 25, "The maximum QPS to the Kubernetes API server")
	flag.IntVar(&k8sClientBurst, "k8s-client-burst", 100, "The maximum burst for throttle to the Kubernetes API server")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctrl.Log.V(1).Info("Test debug")

	config := ctrl.GetConfigOrDie()
	config.QPS = float32(k8sClientQPS)
	config.Burst = k8sClientBurst

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "cilium-ha-egress.angeloxx.ch",

		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.HAEgressIPReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("HAEgressIP"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("cilium-ha-egress"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HAEgressIP")
		os.Exit(1)
	}
	if err = (&controllers.ServicesController{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Services"),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("cilium-ha-egress"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Services")
		os.Exit(1)
	}
	if err = (&controllers.CiliumEgressGatewayPolicyReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("CiliumEgressGatewayPolicy"),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("cilium-ha-egress"),
		CiliumNamespace: ciliumNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller CiliumEgressGatewayPolicy")
		os.Exit(1)
	}
	if err = (&controllers.LeasesController{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("Leases"),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("cilium-ha-egress"),
		CiliumNamespace: ciliumNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller Leases")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}
