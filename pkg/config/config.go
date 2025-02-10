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

package config

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/kueue/apis/config/v1beta1"
)

type OperatorConfig struct {
	AppWrapper        *AppWrapperConfig        `json:"appwrapper,omitempty"`
	CertManagement    *CertManagementConfig    `json:"certManagement,omitempty"`
	ControllerManager *ControllerManagerConfig `json:"controllerManager,omitempty"`
	WebhooksEnabled   *bool                    `json:"webhooksEnabled,omitempty"`
}

type AppWrapperConfig struct {
	EnableKueueIntegrations bool                       `json:"enableKueueIntegrations,omitempty"`
	KueueJobReconciller     *KueueJobReconcillerConfig `json:"kueueJobReconciller,omitempty"`
	Autopilot               *AutopilotConfig           `json:"autopilot,omitempty"`
	UserRBACAdmissionCheck  bool                       `json:"userRBACAdmissionCheck,omitempty"`
	FaultTolerance          *FaultToleranceConfig      `json:"faultTolerance,omitempty"`
	SchedulerName           string                     `json:"schedulerName,omitempty"`
	DefaultQueueName        string                     `json:"defaultQueueName,omitempty"`
	SlackQueueName          string                     `json:"slackQueueName,omitempty"`
}

type KueueJobReconcillerConfig struct {
	ManageJobsWithoutQueueName  bool                      `json:"manageJobsWithoutQueueName,omitempty"`
	ManageJobsNamespaceSelector *metav1.LabelSelector     `json:"manageJobsNamespaceSelector,omitempty"`
	WaitForPodsReady            *v1beta1.WaitForPodsReady `json:"waitForPodsReady,omitempty"`
	LabelKeysToCopy             []string                  `json:"labelKeysToCopy,omitempty"`
}

type AutopilotConfig struct {
	InjectAntiAffinities   bool                  `json:"injectAntiAffinities,omitempty"`
	MonitorNodes           bool                  `json:"monitorNodes,omitempty"`
	ResourceTaints         map[string][]v1.Taint `json:"resourceTaints,omitempty"`
	PreferNoScheduleWeight *int32                `json:"preferNoScheduleWeight,omitempty"`
}

type FaultToleranceConfig struct {
	AdmissionGracePeriod        time.Duration `json:"admissionGracePeriod,omitempty"`
	WarmupGracePeriod           time.Duration `json:"warmupGracePeriod,omitempty"`
	FailureGracePeriod          time.Duration `json:"failureGracePeriod,omitempty"`
	RetryPausePeriod            time.Duration `json:"resetPause,omitempty"`
	RetryLimit                  int32         `json:"retryLimit,omitempty"`
	ForcefulDeletionGracePeriod time.Duration `json:"deletionGracePeriod,omitempty"`
	GracePeriodMaximum          time.Duration `json:"gracePeriodCeiling,omitempty"`
	SuccessTTL                  time.Duration `json:"successTTLCeiling,omitempty"`
}

type CertManagementConfig struct {
	Namespace                   string `json:"namespace,omitempty"`
	CertificateDir              string `json:"certificateDir,omitempty"`
	CertificateName             string `json:"certificateName,omitempty"`
	CertificateOrg              string `json:"certificateOrg,omitempty"`
	MutatingWebhookConfigName   string `json:"mutatingWebhookConfigName,omitempty"`
	ValidatingWebhookConfigName string `json:"validatingWebhookConfigName,omitempty"`
	WebhookServiceName          string `json:"webhookServiceName,omitempty"`
	WebhookSecretName           string `json:"webhookSecretName,omitempty"`
}

type ControllerManagerConfig struct {
	Metrics        MetricsConfiguration `json:"metrics,omitempty"`
	Health         HealthConfiguration  `json:"health,omitempty"`
	LeaderElection bool                 `json:"leaderElection,omitempty"`
	EnableHTTP2    bool                 `json:"enableHTTP2,omitempty"`
}

type MetricsConfiguration struct {
	BindAddress string `json:"bindAddress,omitempty"`
}

type HealthConfiguration struct {
	BindAddress string `json:"bindAddress,omitempty"`
}

// NewAppWrapperConfig constructs an AppWrapperConfig and fills in default values
func NewAppWrapperConfig() *AppWrapperConfig {
	return &AppWrapperConfig{
		EnableKueueIntegrations: true,
		KueueJobReconciller: &KueueJobReconcillerConfig{
			ManageJobsWithoutQueueName: true,
			ManageJobsNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"kube-system", "kueue-system", "appwrapper-system"},
					},
				},
			},
			WaitForPodsReady: &v1beta1.WaitForPodsReady{Enable: true},
			LabelKeysToCopy:  []string{},
		},
		Autopilot: &AutopilotConfig{
			InjectAntiAffinities: true,
			MonitorNodes:         true,
			ResourceTaints: map[string][]v1.Taint{
				"nvidia.com/gpu": {
					{Key: "autopilot.ibm.com/gpuhealth", Value: "WARN", Effect: v1.TaintEffectPreferNoSchedule},
					{Key: "autopilot.ibm.com/gpuhealth", Value: "TESTING", Effect: v1.TaintEffectNoSchedule},
					{Key: "autopilot.ibm.com/gpuhealth", Value: "EVICT", Effect: v1.TaintEffectNoExecute}},
			},
			PreferNoScheduleWeight: ptr.To(int32(50)),
		},
		UserRBACAdmissionCheck: true,
		FaultTolerance: &FaultToleranceConfig{
			AdmissionGracePeriod:        1 * time.Minute,
			WarmupGracePeriod:           5 * time.Minute,
			FailureGracePeriod:          1 * time.Minute,
			RetryPausePeriod:            90 * time.Second,
			RetryLimit:                  3,
			ForcefulDeletionGracePeriod: 10 * time.Minute,
			GracePeriodMaximum:          24 * time.Hour,
			SuccessTTL:                  7 * 24 * time.Hour,
		},
	}
}

func ValidateAppWrapperConfig(config *AppWrapperConfig) error {
	if config.FaultTolerance.ForcefulDeletionGracePeriod > config.FaultTolerance.GracePeriodMaximum {
		return fmt.Errorf("ForcefulDelectionGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.ForcefulDeletionGracePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.RetryPausePeriod > config.FaultTolerance.GracePeriodMaximum {
		return fmt.Errorf("RetryPausePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.RetryPausePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.FailureGracePeriod > config.FaultTolerance.GracePeriodMaximum {
		return fmt.Errorf("FailureGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.FailureGracePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.AdmissionGracePeriod > config.FaultTolerance.GracePeriodMaximum {
		return fmt.Errorf("AdmissionGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.AdmissionGracePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.WarmupGracePeriod > config.FaultTolerance.GracePeriodMaximum {
		return fmt.Errorf("AdmissionGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.WarmupGracePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.AdmissionGracePeriod > config.FaultTolerance.WarmupGracePeriod {
		return fmt.Errorf("AdmissionGracePeriod %v exceeds AdmissionGracePeriod %v",
			config.FaultTolerance.WarmupGracePeriod, config.FaultTolerance.GracePeriodMaximum)
	}
	if config.FaultTolerance.SuccessTTL <= 0 {
		return fmt.Errorf("SuccessTTL %v is not a positive duration", config.FaultTolerance.SuccessTTL)
	}

	return nil
}

// NewCertManagermentConfig constructs a CertManagementConfig and fills in default values
func NewCertManagementConfig(namespace string) *CertManagementConfig {
	return &CertManagementConfig{
		Namespace:                   namespace,
		CertificateDir:              "/tmp/k8s-webhook-server/serving-certs",
		CertificateName:             "appwrapper-ca",
		CertificateOrg:              "appwrapper",
		MutatingWebhookConfigName:   "appwrapper-mutating-webhook-configuration",
		ValidatingWebhookConfigName: "appwrapper-validating-webhook-configuration",
		WebhookServiceName:          "appwrapper-webhook-service",
		WebhookSecretName:           "appwrapper-webhook-server-cert",
	}
}

// NewControllerRuntimeConfig constructs a ControllerRuntimeConfig and fills in default values
func NewControllerManagerConfig() *ControllerManagerConfig {
	return &ControllerManagerConfig{
		Metrics: MetricsConfiguration{
			BindAddress: ":8443",
		},
		Health: HealthConfiguration{
			BindAddress: ":8081",
		},
		LeaderElection: false,
		EnableHTTP2:    false,
	}
}
