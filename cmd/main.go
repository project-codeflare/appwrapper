/*
Copyright 2024 IBM Corporation.

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
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/yaml"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/pkg/config"
	"github.com/project-codeflare/appwrapper/pkg/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme       = runtime.NewScheme()
	setupLog     = ctrl.Log.WithName("setup")
	BuildVersion = "UNKNOWN"
	BuildDate    = "UNKNOWN"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kueue.AddToScheme(scheme))
	utilruntime.Must(workloadv1beta2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var configMapName string
	flag.StringVar(&configMapName, "config", "appwrapper-operator-config",
		"The name of the ConfigMap to load the operator configuration from. "+
			"If it does not exist, the operator will create and initialise it.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("Build info", "version", BuildVersion, "date", BuildDate)

	namespace, err := getNamespace()
	exitOnError(err, "unable to get operator namespace")

	cfg := &config.OperatorConfig{
		AppWrapper:        config.NewAppWrapperConfig(),
		CertManagement:    config.NewCertManagementConfig(namespace),
		ControllerManager: config.NewControllerManagerConfig(),
	}

	k8sConfig, err := ctrl.GetConfig()
	exitOnError(err, "unable to get client config")
	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	exitOnError(err, "unable to create Kubernetes client")
	ctx := ctrl.SetupSignalHandler()

	cmName := types.NamespacedName{Namespace: namespace, Name: configMapName}
	exitOnError(loadIntoOrCreate(ctx, k8sClient, cmName, cfg), "unable to initialise configuration")

	setupLog.Info("Configuration", "config", cfg)
	exitOnError(config.ValidateAppWrapperConfig(cfg.AppWrapper), "invalid appwrapper config")

	tlsOpts := []func(*tls.Config){}
	if !cfg.ControllerManager.EnableHTTP2 {
		// Unless EnableHTTP2 was set to True, http/2 should be disabled
		// due to its vulnerabilities. More specifically, disabling http/2 will
		// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
		// Rapid Reset CVEs. For more information see:
		// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
		// - https://github.com/advisories/GHSA-4374-p667-p6c8
		disableHTTP2 := func(c *tls.Config) {
			setupLog.Info("disabling http/2")
			c.NextProtos = []string{"http/1.1"}
		}
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	mgr, err := ctrl.NewManager(k8sConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   cfg.ControllerManager.Metrics.BindAddress,
			SecureServing: cfg.ControllerManager.Metrics.SecureServing,
			TLSOpts:       tlsOpts,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			TLSOpts: tlsOpts,
		}),
		HealthProbeBindAddress: cfg.ControllerManager.Health.BindAddress,
		LeaderElection:         cfg.ControllerManager.LeaderElection,
		LeaderElectionID:       "f134c674.codeflare.dev",
	})
	exitOnError(err, "unable to start manager")

	certsReady := make(chan struct{})

	if os.Getenv("ENABLE_WEBHOOKS") == "false" {
		close(certsReady)
	} else {
		exitOnError(controller.SetupCertManagement(mgr, cfg.CertManagement, certsReady), "Unable to set up cert rotation")
	}

	// Ascynchronous because controllers need to wait for certificate to be ready for webhooks to work
	go controller.SetupControllers(ctx, mgr, cfg.AppWrapper, certsReady, setupLog)

	exitOnError(controller.SetupIndexers(ctx, mgr, cfg.AppWrapper), "unable to setup indexers")
	exitOnError(controller.SetupProbeEndpoints(mgr, certsReady), "unable to setup probe endpoints")

	setupLog.Info("starting manager")
	exitOnError(mgr.Start(ctx), "problem running manager")
}

func getNamespace() (string, error) {
	// This way assumes you've set the NAMESPACE environment variable either manually, when running
	// the operator standalone, or using the downward API, when running the operator in-cluster.
	if ns := os.Getenv("NAMESPACE"); ns != "" {
		return ns, nil
	}

	// Fall back to the namespace associated with the service account token, if available
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns, nil
		}
	}

	return "", fmt.Errorf("unable to determine current namespace")
}

func loadIntoOrCreate(ctx context.Context, k8sClient client.Client, cmName types.NamespacedName,
	cfg *config.OperatorConfig) error {
	configMap := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, cmName, configMap)
	if apierrors.IsNotFound(err) {
		if content, err := yaml.Marshal(cfg); err == nil {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: cmName.Name, Namespace: cmName.Namespace},
				Data:       map[string]string{"config.yaml": string(content)},
			}
			return k8sClient.Create(ctx, configMap)
		} else {
			return err
		}
	} else if err != nil {
		return err
	}

	if len(configMap.Data) != 1 {
		return fmt.Errorf("cannot resolve config from ConfigMap %s/%s", configMap.Namespace, configMap.Name)
	}

	for _, data := range configMap.Data {
		return yaml.Unmarshal([]byte(data), cfg)
	}

	return nil
}

func exitOnError(err error, msg string) {
	if err != nil {
		setupLog.Error(err, msg)
		os.Exit(1)
	}
}
