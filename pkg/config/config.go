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
)

type OperatorConfig struct {
	AppWrapper        *AppWrapperConfig        `json:"appwrapper,omitempty"`
	CertManagement    *CertManagementConfig    `json:"certManagement,omitempty"`
	ControllerManager *ControllerManagerConfig `json:"controllerManager,omitempty"`
	WebhooksEnabled   *bool                    `json:"webhooksEnabled,omitempty"`
}

type AppWrapperConfig struct {
	ManageJobsWithoutQueueName bool                  `json:"manageJobsWithoutQueueName,omitempty"`
	EnableKueueIntegrations    bool                  `json:"enableKueueIntegrations,omitempty"`
	DisableChildAdmissionCtrl  bool                  `json:"disableChildAdmissionCtrl,omitempty"`
	FaultTolerance             *FaultToleranceConfig `json:"faultTolerance,omitempty"`
}

type FaultToleranceConfig struct {
	WarmupGracePeriod   time.Duration `json:"warmupGracePeriod,omitempty"`
	FailureGracePeriod  time.Duration `json:"failureGracePeriod,omitempty"`
	ResetPause          time.Duration `json:"resetPause,omitempty"`
	RetryLimit          int32         `json:"retryLimit,omitempty"`
	DeletionGracePeriod time.Duration `json:"deletionGracePeriod,omitempty"`
	GracePeriodCeiling  time.Duration `json:"gracePeriodCeiling,omitempty"`
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
	BindAddress   string `json:"bindAddress,omitempty"`
	SecureServing bool   `json:"secureServing,omitempty"`
}

type HealthConfiguration struct {
	BindAddress string `json:"bindAddress,omitempty"`
}

// NewAppWrapperConfig constructs an AppWrapperConfig and fills in default values
func NewAppWrapperConfig() *AppWrapperConfig {
	return &AppWrapperConfig{
		ManageJobsWithoutQueueName: true,
		EnableKueueIntegrations:    true,
		DisableChildAdmissionCtrl:  false,
		FaultTolerance: &FaultToleranceConfig{
			WarmupGracePeriod:   5 * time.Minute,
			FailureGracePeriod:  1 * time.Minute,
			ResetPause:          90 * time.Second,
			RetryLimit:          3,
			DeletionGracePeriod: 10 * time.Minute,
			GracePeriodCeiling:  24 * time.Hour,
		},
	}
}

func ValidateAppWrapperConfig(config *AppWrapperConfig) error {
	if config.FaultTolerance.DeletionGracePeriod > config.FaultTolerance.GracePeriodCeiling {
		return fmt.Errorf("DelectionGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.DeletionGracePeriod, config.FaultTolerance.GracePeriodCeiling)
	}
	if config.FaultTolerance.ResetPause > config.FaultTolerance.GracePeriodCeiling {
		return fmt.Errorf("ResetPause %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.ResetPause, config.FaultTolerance.GracePeriodCeiling)
	}
	if config.FaultTolerance.FailureGracePeriod > config.FaultTolerance.GracePeriodCeiling {
		return fmt.Errorf("FailureGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.FailureGracePeriod, config.FaultTolerance.GracePeriodCeiling)
	}
	if config.FaultTolerance.WarmupGracePeriod > config.FaultTolerance.GracePeriodCeiling {
		return fmt.Errorf("WarmupGracePeriod %v exceeds GracePeriodCeiling %v",
			config.FaultTolerance.WarmupGracePeriod, config.FaultTolerance.GracePeriodCeiling)
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

// NewControllerRuntimeConfig constructs a ControllerRuntimeConfig and filles in default values
func NewControllerManagerConfig() *ControllerManagerConfig {
	return &ControllerManagerConfig{
		Metrics: MetricsConfiguration{
			BindAddress:   ":8080",
			SecureServing: false,
		},
		Health: HealthConfiguration{
			BindAddress: ":8081",
		},
		LeaderElection: false,
		EnableHTTP2:    false,
	}
}
