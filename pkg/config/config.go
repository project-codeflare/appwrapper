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

import "time"

type AppWrapperConfig struct {
	ManageJobsWithoutQueueName bool                 `json:"manageJobsWithoutQueueName,omitempty"`
	StandaloneMode             bool                 `json:"standaloneMode,omitempty"`
	FaultTolerance             FaultToleranceConfig `json:"faultTolerance,omitempty"`
	CertManagement             CertManagementConfig `json:"certManagement,omitempty"`
}

type FaultToleranceConfig struct {
	WarmupGracePeriod  time.Duration `json:"warmupGracePeriod,omitempty"`
	FailureGracePeriod time.Duration `json:"failureGracePeriod,omitempty"`
	ResetPause         time.Duration `json:"resetPause,omitempty"`
	RetryLimit         int32         `json:"retryLimit,omitempty"`
}

type CertManagementConfig struct {
	WebhookServiceName string `json:"webhookServiceName,omitempty"`
	WebhookSecretName  string `json:"webhookSecretName,omitempty"`
	Namespace          string `json:"namespace,omitempty"`
}

// NewConfig constructs an AppWrapperConfig and fills in default values
func NewConfig(namespace string) *AppWrapperConfig {
	return &AppWrapperConfig{
		ManageJobsWithoutQueueName: true,
		StandaloneMode:             false,
		FaultTolerance: FaultToleranceConfig{
			WarmupGracePeriod:  5 * time.Minute,
			FailureGracePeriod: 1 * time.Minute,
			ResetPause:         90 * time.Second,
			RetryLimit:         3,
		},
		CertManagement: CertManagementConfig{
			WebhookServiceName: "webhook-service",
			WebhookSecretName:  "webhook-server-cert",
			Namespace:          namespace,
		},
	}
}
