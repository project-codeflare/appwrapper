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

package appwrapper

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	"sigs.k8s.io/kueue/pkg/podset"
	utilslices "sigs.k8s.io/kueue/pkg/util/slices"

	awv1beta2 "github.com/project-codeflare/appwrapper/api/v1beta2"
	"github.com/project-codeflare/appwrapper/internal/controller/workload"
	"github.com/project-codeflare/appwrapper/pkg/config"
	"github.com/project-codeflare/appwrapper/pkg/utils"
)

var _ = Describe("AppWrapper Controller", func() {
	var awReconciler *AppWrapperReconciler
	var awName types.NamespacedName
	markerPodSet := podset.PodSetInfo{
		Labels:          map[string]string{"testkey1": "value1"},
		Annotations:     map[string]string{"test2": "test2"},
		NodeSelector:    map[string]string{"nodeName": "myNode"},
		Tolerations:     []v1.Toleration{{Key: "aKey", Operator: "Exists", Effect: "NoSchedule"}},
		SchedulingGates: []v1.PodSchedulingGate{{Name: "aGate"}},
	}
	var kueuePodSets []kueue.PodSet

	advanceToResuming := func(components ...awv1beta2.AppWrapperComponent) {
		By("Create an AppWrapper")
		aw := toAppWrapper(components...)
		aw.Spec.Suspend = true
		Expect(k8sClient.Create(ctx, aw)).To(Succeed())
		awName = types.NamespacedName{
			Name:      aw.Name,
			Namespace: aw.Namespace,
		}
		awConfig := config.NewAppWrapperConfig()
		awConfig.FaultTolerance.FailureGracePeriod = 0 * time.Second
		awConfig.FaultTolerance.RetryPausePeriod = 0 * time.Second
		awConfig.FaultTolerance.RetryLimit = 0
		awConfig.FaultTolerance.SuccessTTL = 0 * time.Second
		awConfig.Autopilot.ResourceTaints["nvidia.com/gpu"] = append(awConfig.Autopilot.ResourceTaints["nvidia.com/gpu"], v1.Taint{Key: "extra", Value: "test", Effect: v1.TaintEffectNoExecute})
		awConfig.Autopilot.ResourceTaints["nvidia.com/gpu"] = append(awConfig.Autopilot.ResourceTaints["nvidia.com/gpu"], v1.Taint{Key: "extra2", Value: "test2", Effect: v1.TaintEffectPreferNoSchedule})

		awReconciler = &AppWrapperReconciler{
			Client:   k8sClient,
			Recorder: &record.FakeRecorder{},
			Scheme:   k8sClient.Scheme(),
			Config:   awConfig,
		}
		kueuePodSets = (*workload.AppWrapper)(aw).PodSets()

		By("Reconciling: Empty -> Suspended")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperSuspended))

		By("Updating aw.Spec by invoking RunWithPodSetsInfo")
		Expect((*workload.AppWrapper)(aw).RunWithPodSetsInfo([]podset.PodSetInfo{markerPodSet, markerPodSet})).To(Succeed())
		Expect(aw.Spec.Suspend).To(BeFalse())
		Expect(k8sClient.Update(ctx, aw)).To(Succeed())

		By("Reconciling: Suspended -> Resuming")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperResuming))
		Expect(controllerutil.ContainsFinalizer(aw, AppWrapperFinalizer)).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
	}

	beginRunning := func() {
		By("Reconciling: Resuming -> Running")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw := getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.PodsReady))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(utils.ExpectedPodCount(aw)).Should(Equal(podStatus.pending))

		By("Simulating first Pod Running")
		Expect(setPodStatus(aw, v1.PodRunning, 1)).To(Succeed())
		By("Reconciling: Running -> Running")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.PodsReady))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		podStatus, err = awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.running).Should(Equal(int32(1)))
		Expect(utils.ExpectedPodCount(aw)).Should(Equal(podStatus.pending + podStatus.running))
	}

	fullyRunning := func() {
		aw := getAppWrapper(awName)
		By("Simulating all Pods Running")
		pc, err := utils.ExpectedPodCount(aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(setPodStatus(aw, v1.PodRunning, pc)).To(Succeed())
		By("Reconciling: Running -> Running")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.PodsReady))).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).PodsReady()).Should(BeTrue())
		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.running).Should(Equal(pc))
		_, _, finished := (*workload.AppWrapper)(aw).Finished()
		Expect(finished).Should(BeFalse())
	}

	validateMarkers := func(p *v1.Pod) {
		for k, v := range markerPodSet.Annotations {
			Expect(p.Annotations).Should(HaveKeyWithValue(k, v))
		}
		for k, v := range markerPodSet.Labels {
			Expect(p.Labels).Should(HaveKeyWithValue(k, v))
		}
		for _, v := range markerPodSet.Tolerations {
			Expect(p.Spec.Tolerations).Should(ContainElement(v))
		}
		for k, v := range markerPodSet.NodeSelector {
			Expect(p.Spec.NodeSelector).Should(HaveKeyWithValue(k, v))
		}
		for _, v := range markerPodSet.SchedulingGates {
			Expect(p.Spec.SchedulingGates).Should(ContainElement(v))
		}
	}

	validateAutopilot := func(p *v1.Pod) {
		if p.Spec.Containers[0].Resources.Requests.Name("nvidia.com/gpu", resource.DecimalSI).IsZero() {
			Expect(p.Spec.Affinity).Should(BeNil())
		} else {
			Expect(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(BeNil())
			Expect(p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms).Should(HaveLen(1))
			mes := p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
			for _, taint := range awReconciler.Config.Autopilot.ResourceTaints["nvidia.com/gpu"] {
				found := false
				if taint.Effect == v1.TaintEffectNoExecute || taint.Effect == v1.TaintEffectNoSchedule {
					for _, me := range mes {
						if me.Key == taint.Key {
							Expect(me.Operator).Should(Equal(v1.NodeSelectorOpNotIn))
							Expect(me.Values).Should(ContainElement(taint.Value))
							found = true
						}
					}
				} else {
					for _, st := range p.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
						for _, me := range st.Preference.MatchExpressions {
							if me.Key == taint.Key {
								Expect(me.Operator).Should(Equal(v1.NodeSelectorOpNotIn))
								Expect(me.Values).Should(ContainElement(taint.Value))
								found = true
							}
						}
					}
				}
				Expect(found).Should(BeTrue())
			}
		}
	}

	AfterEach(func() {
		By("Cleanup the AppWrapper and ensure no Pods remain")
		aw := &awv1beta2.AppWrapper{}
		Expect(k8sClient.Get(ctx, awName, aw)).To(Succeed())
		Expect(k8sClient.Delete(ctx, aw)).To(Succeed())

		By("Reconciling: Deletion processing")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // initiate deletion
		Expect(err).NotTo(HaveOccurred())
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // see deletion has completed
		Expect(err).NotTo(HaveOccurred())

		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.failed + podStatus.succeeded + podStatus.running + podStatus.pending).Should(Equal(int32(0)))
	})

	It("Happy Path Lifecycle", func() {
		advanceToResuming(pod(100, 1, true), pod(100, 0, false))
		beginRunning()
		fullyRunning()

		By("Simulating one Pod Completing")
		aw := getAppWrapper(awName)
		Expect(setPodStatus(aw, v1.PodSucceeded, 1)).To(Succeed())
		By("Reconciling: Running -> Running")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperRunning))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		pc, err := utils.ExpectedPodCount(aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(pc).Should(Equal(int32(2)))
		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.running).Should(Equal(int32(1)))
		Expect(podStatus.succeeded).Should(Equal(int32(1)))

		By("Simulating all Pods Completing")
		Expect(setPodStatus(aw, v1.PodSucceeded, 2)).To(Succeed())
		By("Reconciling: Running -> Succeeded")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperSucceeded))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		_, _, finished := (*workload.AppWrapper)(aw).Finished()
		Expect(finished).Should(BeTrue())

		By("Resources are Removed after TTL expires")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())
		aw = getAppWrapper(awName)
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeFalse())
	})

	It("Running Workloads can be Suspended", func() {
		advanceToResuming(pod(100, 0, false), pod(100, 1, true))
		beginRunning()
		fullyRunning()

		By("Invoking Suspend and RestorePodSetsInfo")
		aw := getAppWrapper(awName)
		(*workload.AppWrapper)(aw).Suspend()
		Expect((*workload.AppWrapper)(aw).RestorePodSetsInfo(utilslices.Map(kueuePodSets, podset.FromPodSet))).To(BeTrue())
		Expect(k8sClient.Update(ctx, aw)).To(Succeed())

		By("Reconciling: Running -> Suspending")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperSuspending))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeTrue())

		By("Reconciling: Suspending -> Suspended")
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // initiate deletion
		Expect(err).NotTo(HaveOccurred())
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // see deletion has completed
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperSuspended))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeFalse())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeTrue())
		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.failed + podStatus.succeeded + podStatus.running + podStatus.pending).Should(Equal(int32(0)))
	})

	It("A Pod Failure leads to a failed AppWrapper", func() {
		advanceToResuming(pod(100, 0, false), pod(100, 0, true))
		beginRunning()
		fullyRunning()

		By("Simulating one Pod Failing")
		aw := getAppWrapper(awName)
		Expect(setPodStatus(aw, v1.PodFailed, 1)).To(Succeed())

		By("Reconciling: Running -> Failed")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) //  detect failure
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperFailed))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		_, _, finished := (*workload.AppWrapper)(aw).Finished()
		Expect(finished).Should(BeFalse())

		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // initiate deletion
		Expect(err).NotTo(HaveOccurred())
		_, err = awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName}) // see deletion has completed
		Expect(err).NotTo(HaveOccurred())

		aw = getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperFailed))

		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeFalse())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		_, _, finished = (*workload.AppWrapper)(aw).Finished()
		Expect(finished).Should(BeTrue())
	})

	It("Failure during resource creation leads to a failed AppWrapper", func() {
		advanceToResuming(pod(100, 0, false), malformedPod(100))

		By("Reconciling: Resuming -> Failed")
		_, err := awReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: awName})
		Expect(err).NotTo(HaveOccurred())

		aw := getAppWrapper(awName)
		Expect(aw.Status.Phase).Should(Equal(awv1beta2.AppWrapperFailed))
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.ResourcesDeployed))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.QuotaReserved))).Should(BeTrue())
		Expect(meta.IsStatusConditionTrue(aw.Status.Conditions, string(awv1beta2.PodsReady))).Should(BeFalse())
		Expect((*workload.AppWrapper)(aw).IsActive()).Should(BeTrue())
		Expect((*workload.AppWrapper)(aw).IsSuspended()).Should(BeFalse())
		podStatus, err := awReconciler.getPodStatus(ctx, aw)
		Expect(err).NotTo(HaveOccurred())
		Expect(podStatus.pending).Should(Equal(int32(1)))
	})

	It("Validating PodSet Injection invariants on minimal pods", func() {
		advanceToResuming(pod(100, 0, false), pod(100, 1, true))
		beginRunning()
		aw := getAppWrapper(awName)
		pods := getPods(aw)
		Expect(pods).Should(HaveLen(2))

		By("Validate expected markers and Autopilot anti-affinities were injected")
		for _, p := range pods {
			Expect(p.Labels).Should(HaveKeyWithValue(awv1beta2.AppWrapperLabel, awName.Name))
			validateMarkers(&p)
			validateAutopilot(&p)
		}
	})

	It("Validating PodSet Injection invariants on complex pods", func() {
		advanceToResuming(complexPodYaml(), complexPodYaml())
		beginRunning()
		aw := getAppWrapper(awName)
		pods := getPods(aw)
		Expect(pods).Should(HaveLen(2))

		By("Validate expected markers and Autopilot anti-affinities were injected")
		for _, p := range pods {
			Expect(p.Labels).Should(HaveKeyWithValue(awv1beta2.AppWrapperLabel, awName.Name))
			validateMarkers(&p)
			validateAutopilot(&p)
		}

		By("Validate complex pod elements were not removed")
		for _, p := range pods {
			Expect(p.Labels).Should(HaveKeyWithValue("myComplexLabel", "myComplexValue"))
			Expect(p.Annotations).Should(HaveKeyWithValue("myComplexAnnotation", "myComplexValue"))
			Expect(p.Spec.NodeSelector).Should(HaveKeyWithValue("myComplexSelector", "myComplexValue"))
			Expect(p.Spec.Tolerations).Should(ContainElement(v1.Toleration{Key: "myComplexKey", Value: "myComplexValue", Operator: v1.TolerationOpEqual, Effect: v1.TaintEffectNoSchedule}))
			Expect(p.Spec.SchedulingGates).Should(ContainElement(v1.PodSchedulingGate{Name: "myComplexGate"}))
			mes := p.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
			found := false
			for _, me := range mes {
				if me.Key == "kubernetes.io/hostname" {
					Expect(me.Operator).Should(Equal(v1.NodeSelectorOpNotIn))
					Expect(me.Values).Should(ContainElement("badHost1"))
					found = true
				}
			}
			Expect(found).Should(BeTrue())
		}
	})
})

var _ = Describe("AppWrapper Annotations", func() {
	var awReconciler *AppWrapperReconciler

	BeforeEach(func() {
		awReconciler = &AppWrapperReconciler{
			Client:   k8sClient,
			Recorder: &record.FakeRecorder{},
			Scheme:   k8sClient.Scheme(),
			Config:   config.NewAppWrapperConfig(),
		}
	})

	It("Unannotated appwrappers use defaults", func() {
		aw := &awv1beta2.AppWrapper{}
		Expect(awReconciler.admissionGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.AdmissionGracePeriod))
		Expect(awReconciler.warmupGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.WarmupGracePeriod))
		Expect(awReconciler.failureGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.FailureGracePeriod))
		Expect(awReconciler.retryLimit(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.RetryLimit))
		Expect(awReconciler.retryPauseDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.RetryPausePeriod))
		Expect(awReconciler.forcefulDeletionGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.ForcefulDeletionGracePeriod))
		Expect(awReconciler.deletionOnFailureGraceDuration(ctx, aw)).Should(Equal(0 * time.Second))
		Expect(awReconciler.timeToLiveAfterSucceededDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.SuccessTTL))
	})

	It("Valid annotations override defaults", func() {
		allowed := 10 * time.Second
		aw := &awv1beta2.AppWrapper{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					awv1beta2.AdmissionGracePeriodDurationAnnotation: allowed.String(),
					awv1beta2.WarmupGracePeriodDurationAnnotation:    allowed.String(),
					awv1beta2.FailureGracePeriodDurationAnnotation:   allowed.String(),
					awv1beta2.RetryPausePeriodDurationAnnotation:     allowed.String(),
					awv1beta2.RetryLimitAnnotation:                   "101",
					awv1beta2.ForcefulDeletionGracePeriodAnnotation:  allowed.String(),
					awv1beta2.DeletionOnFailureGracePeriodAnnotation: allowed.String(),
					awv1beta2.SuccessTTLAnnotation:                   allowed.String(),
				},
			},
		}
		Expect(awReconciler.admissionGraceDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.warmupGraceDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.failureGraceDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.retryLimit(ctx, aw)).Should(Equal(int32(101)))
		Expect(awReconciler.retryPauseDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.forcefulDeletionGraceDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.deletionOnFailureGraceDuration(ctx, aw)).Should(Equal(allowed))
		Expect(awReconciler.timeToLiveAfterSucceededDuration(ctx, aw)).Should(Equal(allowed))
	})

	It("Malformed annotations use defaults", func() {
		malformed := "222badTime"
		aw := &awv1beta2.AppWrapper{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					awv1beta2.AdmissionGracePeriodDurationAnnotation: malformed,
					awv1beta2.WarmupGracePeriodDurationAnnotation:    malformed,
					awv1beta2.FailureGracePeriodDurationAnnotation:   malformed,
					awv1beta2.RetryPausePeriodDurationAnnotation:     malformed,
					awv1beta2.RetryLimitAnnotation:                   "abc",
					awv1beta2.ForcefulDeletionGracePeriodAnnotation:  malformed,
					awv1beta2.DeletionOnFailureGracePeriodAnnotation: malformed,
					awv1beta2.SuccessTTLAnnotation:                   malformed,
				},
			},
		}
		Expect(awReconciler.admissionGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.AdmissionGracePeriod))
		Expect(awReconciler.warmupGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.WarmupGracePeriod))
		Expect(awReconciler.failureGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.FailureGracePeriod))
		Expect(awReconciler.retryLimit(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.RetryLimit))
		Expect(awReconciler.retryPauseDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.RetryPausePeriod))
		Expect(awReconciler.forcefulDeletionGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.ForcefulDeletionGracePeriod))
		Expect(awReconciler.deletionOnFailureGraceDuration(ctx, aw)).Should(Equal(0 * time.Second))
		Expect(awReconciler.timeToLiveAfterSucceededDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.SuccessTTL))
	})

	It("Out of bounds annotations are clipped", func() {
		negative := -10 * time.Minute
		tooLong := 2 * awReconciler.Config.FaultTolerance.GracePeriodMaximum
		aw := &awv1beta2.AppWrapper{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					awv1beta2.AdmissionGracePeriodDurationAnnotation: negative.String(),
					awv1beta2.WarmupGracePeriodDurationAnnotation:    tooLong.String(),
					awv1beta2.FailureGracePeriodDurationAnnotation:   tooLong.String(),
					awv1beta2.RetryPausePeriodDurationAnnotation:     negative.String(),
					awv1beta2.ForcefulDeletionGracePeriodAnnotation:  tooLong.String(),
					awv1beta2.DeletionOnFailureGracePeriodAnnotation: tooLong.String(),
					awv1beta2.SuccessTTLAnnotation:                   (awReconciler.Config.FaultTolerance.SuccessTTL + 10*time.Second).String(),
				},
			},
		}
		Expect(awReconciler.admissionGraceDuration(ctx, aw)).Should(Equal(0 * time.Second))
		Expect(awReconciler.warmupGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.GracePeriodMaximum))
		Expect(awReconciler.failureGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.GracePeriodMaximum))
		Expect(awReconciler.retryPauseDuration(ctx, aw)).Should(Equal(0 * time.Second))
		Expect(awReconciler.forcefulDeletionGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.GracePeriodMaximum))
		Expect(awReconciler.deletionOnFailureGraceDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.GracePeriodMaximum))
		Expect(awReconciler.timeToLiveAfterSucceededDuration(ctx, aw)).Should(Equal(awReconciler.Config.FaultTolerance.SuccessTTL))
	})

	It("Parsing of terminal exits codes", func() {
		aw := &awv1beta2.AppWrapper{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					awv1beta2.TerminalExitCodesAnnotation:  "3,10,abc,42",
					awv1beta2.RetryableExitCodesAnnotation: "x,10,20",
				},
			},
		}
		Expect(awReconciler.terminalExitCodes(ctx, aw)).Should(Equal([]int{3, 10, 42}))
		Expect(awReconciler.retryableExitCodes(ctx, aw)).Should(Equal([]int{10, 20}))
	})
})
