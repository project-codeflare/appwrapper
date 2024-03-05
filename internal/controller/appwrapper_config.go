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
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kueue/pkg/controller/jobframework"
)

type AppWrapperConfig struct {
	ManageJobsWithoutQueueName bool   `json:"manageJobsWithoutQueueName,omitempty"`
	ServiceAccountName         string `json:"serviceAccountName,omitempty"`
}

// SetupWithManager creates and configures all components of the AppWrapper controller
func SetupWithManager(ctx context.Context, mgr ctrl.Manager, config *AppWrapperConfig) error {
	if err := WorkloadReconciler(
		mgr.GetClient(),
		mgr.GetEventRecorderFor("kueue"),
		jobframework.WithManageJobsWithoutQueueName(config.ManageJobsWithoutQueueName),
	).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("workload controller: %w", err)
	}

	if err := (&AppWrapperReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: config,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("appwrapper controller: %w", err)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := (&AppWrapperWebhook{
			Config: config,
		}).SetupWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("appwrapper webhook: %w", err)
		}
	}

	if err := jobframework.SetupWorkloadOwnerIndex(ctx, mgr.GetFieldIndexer(), GVK); err != nil {
		return fmt.Errorf("appwrapper indexer: %w", err)
	}

	return nil
}
