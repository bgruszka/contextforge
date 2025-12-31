package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = Describe("Sidecar Injection", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("when pod has ctxforge.io/enabled annotation", func() {
		It("should inject the proxy sidecar container", func() {
			podName := "test-injection-enabled"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"ctxforge.io/enabled": "true",
						"ctxforge.io/headers": "x-request-id,x-tenant-id",
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

			// Verify sidecar was injected
			Expect(createdPod.Spec.Containers).To(HaveLen(2), "Expected 2 containers (app + sidecar)")

			// Find the sidecar container
			var sidecar *corev1.Container
			for i := range createdPod.Spec.Containers {
				if createdPod.Spec.Containers[i].Name == "ctxforge-proxy" {
					sidecar = &createdPod.Spec.Containers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil(), "Sidecar container should exist")
			Expect(sidecar.Ports).To(HaveLen(1))
			Expect(sidecar.Ports[0].ContainerPort).To(Equal(int32(9090)))

			// Verify HEADERS_TO_PROPAGATE env var
			var headersEnv *corev1.EnvVar
			for i := range sidecar.Env {
				if sidecar.Env[i].Name == "HEADERS_TO_PROPAGATE" {
					headersEnv = &sidecar.Env[i]
					break
				}
			}
			Expect(headersEnv).NotTo(BeNil())
			Expect(headersEnv.Value).To(Equal("x-request-id,x-tenant-id"))

			// Verify app container has HTTP_PROXY env var
			var appContainer *corev1.Container
			for i := range createdPod.Spec.Containers {
				if createdPod.Spec.Containers[i].Name == "app" {
					appContainer = &createdPod.Spec.Containers[i]
					break
				}
			}
			Expect(appContainer).NotTo(BeNil())

			var httpProxy *corev1.EnvVar
			for i := range appContainer.Env {
				if appContainer.Env[i].Name == "HTTP_PROXY" {
					httpProxy = &appContainer.Env[i]
					break
				}
			}
			Expect(httpProxy).NotTo(BeNil())
			Expect(httpProxy.Value).To(Equal("http://localhost:9090"))

			// Cleanup
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when pod does not have ctxforge.io/enabled annotation", func() {
		It("should not inject the sidecar", func() {
			podName := "test-injection-disabled"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:alpine",
						},
					},
				},
			}

			createdPod, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify no sidecar was injected
			Expect(createdPod.Spec.Containers).To(HaveLen(1), "Expected only 1 container (no sidecar)")

			// Cleanup
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when pod has ctxforge.io/enabled=false annotation", func() {
		It("should not inject the sidecar", func() {
			podName := "test-injection-explicit-false"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"ctxforge.io/enabled": "false",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:alpine",
						},
					},
				},
			}

			createdPod, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify no sidecar was injected
			Expect(createdPod.Spec.Containers).To(HaveLen(1), "Expected only 1 container (no sidecar)")

			// Cleanup
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when pod already has the sidecar", func() {
		It("should not duplicate the sidecar", func() {
			podName := "test-injection-already-injected"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"ctxforge.io/enabled": "true",
						"ctxforge.io/headers": "x-request-id",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:alpine",
						},
						{
							Name:  "ctxforge-proxy",
							Image: "contextforge-proxy:latest",
						},
					},
				},
			}

			createdPod, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify sidecar count is still 2 (not 3)
			Expect(createdPod.Spec.Containers).To(HaveLen(2), "Expected 2 containers (no duplicate sidecar)")

			// Cleanup
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Pod Readiness", func() {
	Context("when sidecar is injected", func() {
		It("should become ready when both containers are healthy", func() {

			podName := "test-readiness"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"ctxforge.io/enabled":     "true",
						"ctxforge.io/headers":     "x-request-id",
						"ctxforge.io/target-port": "80", // nginx listens on port 80
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

			ctx := context.Background()
			_, err := clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for pod to become ready
			err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
				p, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, podName, metav1.GetOptions{})
				if err != nil {
					return false, nil
				}
				for _, cond := range p.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
						return true, nil
					}
				}
				return false, nil
			})
			Expect(err).NotTo(HaveOccurred(), "Pod should become ready")

			// Cleanup
			err = clientset.CoreV1().Pods(testNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
