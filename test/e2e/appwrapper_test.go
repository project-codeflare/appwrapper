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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"

	awv1b2 "github.com/project-codeflare/appwrapper/api/v1beta2"
)

var _ = Describe("AppWrapper E2E Test", func() {
	var appwrappers []*awv1b2.AppWrapper

	BeforeEach(func() {
		appwrappers = []*awv1b2.AppWrapper{}
	})

	AfterEach(func() {
		By("Cleaning up test objects")
		cleanupTestObjects(ctx, appwrappers)
	})

	It("Dummy Test", func() {
		By("Testing nothing of interest...")
	})
})
