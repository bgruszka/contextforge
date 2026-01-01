package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	ctxforgev1alpha1 "github.com/bgruszka/contextforge/api/v1alpha1"
)

var _ = Describe("HeaderPropagationPolicy Controller", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("when a HeaderPropagationPolicy is created", func() {
		It("should update status with matching pods count", func() {
			policyName := "test-controller-policy"
			podName := "test-controller-pod"

			By("creating a HeaderPropagationPolicy")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: testNamespace,
				},
				Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-controller-app",
						},
					},
					PropagationRules: []ctxforgev1alpha1.PropagationRule{
						{
							Headers: []ctxforgev1alpha1.HeaderConfig{
								{Name: "x-request-id"},
								{Name: "x-tenant-id"},
							},
						},
					},
				},
			}
			err := ctxforgeClient.Create(ctx, policy)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the controller to reconcile (initially no pods)")
			err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
				var p ctxforgev1alpha1.HeaderPropagationPolicy
				if err := ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &p); err != nil {
					return false, nil
				}
				// Check that status was updated
				return p.Status.ObservedGeneration > 0, nil
			})
			Expect(err).NotTo(HaveOccurred(), "Controller should reconcile the policy")

			By("verifying Ready condition is False (no matching pods)")
			var updatedPolicy ctxforgev1alpha1.HeaderPropagationPolicy
			err = ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &updatedPolicy)
			Expect(err).NotTo(HaveOccurred())
			readyCondition := meta.FindStatusCondition(updatedPolicy.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("NoMatchingPods"))

			By("creating a pod with matching labels and sidecar injection enabled")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"app": "test-controller-app",
					},
					Annotations: map[string]string{
						"ctxforge.io/enabled":     "true",
						"ctxforge.io/headers":     "x-request-id,x-tenant-id",
						"ctxforge.io/target-port": "80",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:alpine",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80},
							},
						},
					},
				},
			}
			createdPod, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the sidecar was injected")
			Expect(createdPod.Spec.Containers).To(HaveLen(2), "Expected 2 containers (app + sidecar)")

			By("waiting for pod to become Running")
			err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 90*time.Second, true, func(ctx context.Context) (bool, error) {
				p, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
				if err != nil {
					return false, nil
				}
				return p.Status.Phase == corev1.PodRunning, nil
			})
			Expect(err).NotTo(HaveOccurred(), "Pod should become Running")

			By("waiting for controller to update status with matched pod")
			err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				var p ctxforgev1alpha1.HeaderPropagationPolicy
				if err := ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &p); err != nil {
					return false, nil
				}
				return p.Status.AppliedToPods >= 1, nil
			})
			Expect(err).NotTo(HaveOccurred(), "Controller should update AppliedToPods count")

			By("verifying the policy status")
			err = ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &updatedPolicy)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedPolicy.Status.AppliedToPods).To(BeNumerically(">=", 1))

			By("verifying Ready condition is True")
			readyCondition = meta.FindStatusCondition(updatedPolicy.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("PolicyApplied"))

			By("cleaning up the pod")
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("cleaning up the policy")
			err = ctxforgeClient.Delete(ctx, policy)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when no pods match the selector", func() {
		It("should set Ready condition to False with NoMatchingPods reason", func() {
			policyName := "test-no-matching-pods"

			By("creating a HeaderPropagationPolicy with a selector that matches no pods")
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: testNamespace,
				},
				Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "nonexistent-app",
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
			err := ctxforgeClient.Create(ctx, policy)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller to reconcile")
			err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
				var p ctxforgev1alpha1.HeaderPropagationPolicy
				if err := ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &p); err != nil {
					return false, nil
				}
				readyCondition := meta.FindStatusCondition(p.Status.Conditions, "Ready")
				return readyCondition != nil, nil
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying status")
			var updatedPolicy ctxforgev1alpha1.HeaderPropagationPolicy
			err = ctxforgeClient.Get(ctx, types.NamespacedName{Name: policyName, Namespace: testNamespace}, &updatedPolicy)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedPolicy.Status.AppliedToPods).To(Equal(int32(0)))

			readyCondition := meta.FindStatusCondition(updatedPolicy.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("NoMatchingPods"))

			By("cleaning up")
			err = ctxforgeClient.Delete(ctx, policy)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
