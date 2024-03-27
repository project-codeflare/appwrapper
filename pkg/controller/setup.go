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

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/go-logr/logr"
	cert "github.com/open-policy-agent/cert-controller/pkg/rotator"

	"github.com/project-codeflare/appwrapper/internal/controller/appwrapper"
	"github.com/project-codeflare/appwrapper/internal/controller/workload"
	"github.com/project-codeflare/appwrapper/internal/webhook"
	"github.com/project-codeflare/appwrapper/pkg/config"

	"sigs.k8s.io/kueue/pkg/controller/jobframework"
)

const (
	certDir        = "/tmp/k8s-webhook-server/serving-certs"
	vwcName        = "appwrapper-validating-webhook-configuration"
	mwcName        = "appwrapper-mutating-webhook-configuration"
	caName         = "codeflare-ca"
	caOrganization = "codeflare"
)

// SetupControllers creates and configures all components of the AppWrapper controller
func SetupControllers(ctx context.Context, mgr ctrl.Manager, awConfig *config.AppWrapperConfig,
	certsReady chan struct{}, log logr.Logger) {

	log.Info("Waiting for certificates to be generated")
	<-certsReady
	log.Info("Certs ready")

	if !awConfig.StandaloneMode {
		if err := workload.WorkloadReconciler(
			mgr.GetClient(),
			mgr.GetEventRecorderFor("kueue"),
			jobframework.WithManageJobsWithoutQueueName(awConfig.ManageJobsWithoutQueueName),
		).SetupWithManager(mgr); err != nil {
			log.Error(err, "Failed to create workload controller")
			os.Exit(1)
		}

		if err := (&workload.ChildWorkloadReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			log.Error(err, "Failed to create child admission controller")
			os.Exit(1)
		}
	}

	if err := (&appwrapper.AppWrapperReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: awConfig,
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "Failed to create appwrapper controller")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := (&webhook.AppWrapperWebhook{
			Config: awConfig,
		}).SetupWebhookWithManager(mgr); err != nil {
			log.Error(err, "Failed to create webhook")
			os.Exit(1)
		}
	}

	if !awConfig.StandaloneMode {
		if err := jobframework.SetupWorkloadOwnerIndex(ctx, mgr.GetFieldIndexer(), workload.GVK); err != nil {
			log.Error(err, "Failed to create appwrapper indexer")
			os.Exit(1)
		}
	}
}

func SetupProbeEndpoints(mgr ctrl.Manager, certsReady chan struct{}) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
		select {
		case <-certsReady:
			return mgr.GetWebhookServer().StartedChecker()(req)
		default:
			return errors.New("certificates are not ready")
		}
	}); err != nil {
		return fmt.Errorf("readiness check: %w", err)
	}
	return nil
}

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;list;watch;update
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;list;watch;update

func SetupCertManagement(mgr ctrl.Manager, config *config.CertManagementConfig, certsReady chan struct{}) error {
	// DNSName is <service name>.<namespace>.svc
	var dnsName = fmt.Sprintf("%s.%s.svc", config.WebhookServiceName, config.Namespace)

	return cert.AddRotator(mgr, &cert.CertRotator{
		SecretKey:      types.NamespacedName{Namespace: config.Namespace, Name: config.WebhookSecretName},
		CertDir:        certDir,
		CAName:         caName,
		CAOrganization: caOrganization,
		DNSName:        dnsName,
		IsReady:        certsReady,
		Webhooks: []cert.WebhookInfo{
			{Type: cert.Validating, Name: vwcName},
			{Type: cert.Mutating, Name: mwcName},
		},
		// When the controller is running in the leader election mode,
		// we expect webhook server will run in primary and secondary instance
		RequireLeaderElection: false,
	})
}
