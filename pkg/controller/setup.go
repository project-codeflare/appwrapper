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

	cert "github.com/open-policy-agent/cert-controller/pkg/rotator"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/project-codeflare/appwrapper/internal/controller/appwrapper"
	"github.com/project-codeflare/appwrapper/internal/webhook"
	"github.com/project-codeflare/appwrapper/pkg/config"
)

// SetupControllers creates and configures all components of the AppWrapper controller
func SetupControllers(mgr ctrl.Manager, awConfig *config.AppWrapperConfig) error {
	if awConfig.Autopilot != nil && awConfig.Autopilot.MonitorNodes {
		if err := (&appwrapper.NodeHealthMonitor{
			Client: mgr.GetClient(),
			Config: awConfig,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("node health monitor: %w", err)
		}
	}

	if err := (&appwrapper.AppWrapperReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("appwrappers"),
		Scheme:   mgr.GetScheme(),
		Config:   awConfig,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("appwrapper controller: %w", err)
	}

	return nil
}

// SetupWebhooks creates and configures the AppWrapper controller's Webhooks
func SetupWebhooks(mgr ctrl.Manager, awConfig *config.AppWrapperConfig) error {
	if err := webhook.SetupAppWrapperWebhook(mgr, awConfig); err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	return nil
}

func SetupIndexers(ctx context.Context, mgr ctrl.Manager, awConfig *config.AppWrapperConfig) error {
	return nil
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
		CertDir:        config.CertificateDir,
		CAName:         config.CertificateName,
		CAOrganization: config.CertificateOrg,
		DNSName:        dnsName,
		IsReady:        certsReady,
		Webhooks: []cert.WebhookInfo{
			{Type: cert.Validating, Name: config.ValidatingWebhookConfigName},
			{Type: cert.Mutating, Name: config.MutatingWebhookConfigName},
		},
		// When the controller is running in the leader election mode,
		// we expect webhook server will run in primary and secondary instance
		RequireLeaderElection: false,
	})
}
