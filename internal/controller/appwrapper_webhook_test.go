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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workloadv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("AppWrapper Webhook", func() {

	Context("When creating AppWrapper under Defaulting Webhook", func() {
		It("Should fill in the default value of Suspended", func() {
			aw := &workloadv1beta2.AppWrapper{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "webhook",
					Namespace: "default",
				},
				Spec: workloadv1beta2.AppWrapperSpec{
					Suspend:    false,
					Components: []workloadv1beta2.AppWrapperComponent{simplePod(100)},
				},
			}

			Expect(k8sClient.Create(ctx, aw)).To(Succeed())
			Expect(aw.Spec.Suspend).Should(BeTrue(), "aw.Spec.Suspend should have been defaulted to true")
		})
	})

	Context("When creating AppWrapper under Validating Webhook", func() {
		It("Should deny if a required field is empty", func() {

			// TODO(user): Add your logic here

		})

		It("Should admit if all required fields are provided", func() {

			// TODO(user): Add your logic here

		})
	})

})
