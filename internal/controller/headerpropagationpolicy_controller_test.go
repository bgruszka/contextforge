/*
Copyright 2025.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ctxforgev1alpha1 "github.com/bgruszka/contextforge/api/v1alpha1"
)

var _ = Describe("HeaderPropagationPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind HeaderPropagationPolicy")
			headerpropagationpolicy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, headerpropagationpolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &ctxforgev1alpha1.HeaderPropagationPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
						PropagationRules: []ctxforgev1alpha1.PropagationRule{
							{
								Headers: []ctxforgev1alpha1.HeaderConfig{
									{Name: "x-request-id"},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance HeaderPropagationPolicy")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Clean up any pods created during tests
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList, client.InNamespace("default"))).To(Succeed())
			for _, pod := range podList.Items {
				_ = k8sClient.Delete(ctx, &pod)
			}
		})

		It("should successfully reconcile the resource with no matching pods", func() {
			By("Reconciling the created resource")
			controllerReconciler := &HeaderPropagationPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(RequeueAfter))

			By("Verifying the status was updated")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, policy)).To(Succeed())
			Expect(policy.Status.AppliedToPods).To(Equal(int32(0)))
			Expect(policy.Status.ObservedGeneration).To(Equal(policy.Generation))

			By("Verifying the Ready condition is False due to no matching pods")
			readyCondition := meta.FindStatusCondition(policy.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("NoMatchingPods"))
		})

		It("should return no error for deleted resource", func() {
			By("Deleting the resource first")
			resource := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			By("Waiting for deletion to complete")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				return errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*100).Should(BeTrue())

			By("Reconciling the deleted resource")
			controllerReconciler := &HeaderPropagationPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When reconciling with pods matching the selector", func() {
		const policyName = "test-policy-with-pods"
		const podName = "test-pod-with-sidecar"

		ctx := context.Background()

		policyNamespacedName := types.NamespacedName{
			Name:      policyName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating a policy with a pod selector")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: "default",
				},
				Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-app",
						},
					},
					PropagationRules: []ctxforgev1alpha1.PropagationRule{
						{
							Headers: []ctxforgev1alpha1.HeaderConfig{
								{Name: "x-request-id"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			By("creating a pod with the contextforge-proxy sidecar")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: "default",
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main-app",
							Image: "nginx:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("10Mi"),
								},
							},
						},
						{
							Name:  "ctxforge-proxy",
							Image: "ghcr.io/bgruszka/contextforge-proxy:0.1.0",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("10Mi"),
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("updating pod status to Running")
			pod.Status.Phase = corev1.PodRunning
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the policy")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			if err := k8sClient.Get(ctx, policyNamespacedName, policy); err == nil {
				Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
			}

			By("cleaning up the pod")
			pod := &corev1.Pod{}
			podNamespacedName := types.NamespacedName{Name: podName, Namespace: "default"}
			if err := k8sClient.Get(ctx, podNamespacedName, pod); err == nil {
				Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			}
		})

		It("should count matching pods with sidecar", func() {
			By("Reconciling the policy")
			controllerReconciler := &HeaderPropagationPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: policyNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(RequeueAfter))

			By("Verifying the status shows 1 applied pod")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			Expect(k8sClient.Get(ctx, policyNamespacedName, policy)).To(Succeed())
			Expect(policy.Status.AppliedToPods).To(Equal(int32(1)))

			By("Verifying the Ready condition is True")
			readyCondition := meta.FindStatusCondition(policy.Status.Conditions, ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("PolicyApplied"))
		})
	})
})
