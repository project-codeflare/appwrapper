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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "AppWrapperConfig Unit Tests")
}

var _ = Describe("AppWrapper Config", func() {
	It("Config Constructors", func() {
		Expect(NewAppWrapperConfig()).ShouldNot(BeNil())
		Expect(NewCertManagementConfig("testing")).ShouldNot(BeNil())
		Expect(NewControllerManagerConfig()).ShouldNot(BeNil())
	})

	It("Config Validation", func() {
		awc := NewAppWrapperConfig()
		Expect(ValidateAppWrapperConfig(awc)).Should(Succeed())

		bad := &FaultToleranceConfig{ForcefulDeletionGracePeriod: 10 * time.Second, GracePeriodMaximum: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{RetryPausePeriod: 10 * time.Second, GracePeriodMaximum: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{FailureGracePeriod: 10 * time.Second, GracePeriodMaximum: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{AdmissionGracePeriod: 10 * time.Second, GracePeriodMaximum: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{WarmupGracePeriod: 10 * time.Second, GracePeriodMaximum: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{AdmissionGracePeriod: 10 * time.Second, WarmupGracePeriod: 1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())

		bad = &FaultToleranceConfig{SuccessTTL: -1 * time.Second}
		Expect(ValidateAppWrapperConfig(&AppWrapperConfig{FaultTolerance: bad})).ShouldNot(Succeed())
	})
})
