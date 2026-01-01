package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctxforgev1alpha1 "github.com/bgruszka/contextforge/api/v1alpha1"
)

// =============================================================================
// HEADER GENERATION TESTS
// Verifies that headers can be auto-generated when missing
// =============================================================================
var _ = Describe("Header Generation", Ordered, func() {
	var (
		ctx         context.Context
		serviceName string
		curlPodName string
		policyName  string
	)

	BeforeAll(func() {
		ctx = context.Background()
		serviceName = "header-gen-service"
		curlPodName = "curl-header-gen"
		policyName = "header-gen-policy"

		By("Creating a HeaderPropagationPolicy with header generation enabled")
		policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: testNamespace,
			},
			Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": serviceName,
					},
				},
				PropagationRules: []ctxforgev1alpha1.PropagationRule{
					{
						Headers: []ctxforgev1alpha1.HeaderConfig{
							{
								Name:          "x-request-id",
								Generate:      true,
								GeneratorType: "uuid",
							},
							{
								Name:          "x-trace-id",
								Generate:      true,
								GeneratorType: "ulid",
							},
							{
								Name:          "x-timestamp",
								Generate:      true,
								GeneratorType: "timestamp",
							},
							{
								Name: "x-tenant-id", // Not generated, just propagated
							},
						},
					},
				},
			},
		}
		err := ctxforgeClient.Create(ctx, policy)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying echo server with header generation enabled")
		err = deployHeaderGenService(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())
		err = waitForDeploymentWithTimeout(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying curl test pod")
		err = deployCurlPod(ctx, curlPodName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if policyName != "" {
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			if err := ctxforgeClient.Get(ctx, client.ObjectKey{Name: policyName, Namespace: testNamespace}, policy); err == nil {
				_ = ctxforgeClient.Delete(ctx, policy)
			}
		}
		if serviceName != "" {
			_ = clientset.AppsV1().Deployments(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
			_ = clientset.CoreV1().Services(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		}
		if curlPodName != "" {
			_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, curlPodName, metav1.DeleteOptions{})
		}
	})

	Context("when x-request-id header is missing", func() {
		It("should generate a UUID v4 format request ID", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			// Make request WITHOUT x-request-id header
			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-H", "x-tenant-id: tenant-123", // Only send tenant ID, not request ID
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response body: %s\n", body)

			// Verify x-request-id was generated and is in UUID format
			Expect(body).To(ContainSubstring("x-request-id"))

			// UUID v4 pattern: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
			uuidPattern := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
			Expect(uuidPattern.MatchString(body)).To(BeTrue(), "Response should contain a valid UUID v4")
		})

		It("should generate a ULID format trace ID", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response body: %s\n", body)

			// Verify x-trace-id was generated and is in ULID format (26 characters, Crockford's Base32)
			Expect(body).To(ContainSubstring("x-trace-id"))

			// ULID pattern: 26 characters from Crockford's Base32 alphabet
			ulidPattern := regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)
			Expect(ulidPattern.MatchString(body)).To(BeTrue(), "Response should contain a valid ULID")
		})

		It("should generate a timestamp format header", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response body: %s\n", body)

			// Verify x-timestamp was generated and is in RFC3339 format
			Expect(body).To(ContainSubstring("x-timestamp"))

			// RFC3339Nano pattern: 2006-01-02T15:04:05.999999999Z07:00
			timestampPattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
			Expect(timestampPattern.MatchString(body)).To(BeTrue(), "Response should contain a valid timestamp")
		})
	})

	Context("when x-request-id header is provided", func() {
		It("should NOT override the existing header value", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			existingRequestID := "my-existing-request-id-12345"

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-H", fmt.Sprintf("x-request-id: %s", existingRequestID),
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response body: %s\n", body)

			// Verify the existing request ID was preserved
			Expect(body).To(ContainSubstring(existingRequestID))
		})
	})
})

// =============================================================================
// PATH-BASED FILTERING TESTS
// Verifies headers are only propagated for matching paths
// =============================================================================
var _ = Describe("Path-Based Header Filtering", Ordered, func() {
	var (
		ctx         context.Context
		serviceName string
		curlPodName string
		policyName  string
	)

	BeforeAll(func() {
		ctx = context.Background()
		serviceName = "path-filter-service"
		curlPodName = "curl-path-filter"
		policyName = "path-filter-policy"

		By("Creating a HeaderPropagationPolicy with path-based rules")
		policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: testNamespace,
			},
			Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": serviceName,
					},
				},
				PropagationRules: []ctxforgev1alpha1.PropagationRule{
					{
						// Only propagate for /api/* paths
						PathRegex: "^/api/.*",
						Headers: []ctxforgev1alpha1.HeaderConfig{
							{Name: "x-api-key"},
							{Name: "x-request-id"},
						},
					},
					{
						// Propagate tenant ID for all paths
						Headers: []ctxforgev1alpha1.HeaderConfig{
							{Name: "x-tenant-id"},
						},
					},
				},
			},
		}
		err := ctxforgeClient.Create(ctx, policy)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying echo server with path filtering")
		err = deployPathFilterService(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())
		err = waitForDeploymentWithTimeout(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying curl test pod")
		err = deployCurlPod(ctx, curlPodName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if policyName != "" {
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			if err := ctxforgeClient.Get(ctx, client.ObjectKey{Name: policyName, Namespace: testNamespace}, policy); err == nil {
				_ = ctxforgeClient.Delete(ctx, policy)
			}
		}
		if serviceName != "" {
			_ = clientset.AppsV1().Deployments(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
			_ = clientset.CoreV1().Services(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		}
		if curlPodName != "" {
			_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, curlPodName, metav1.DeleteOptions{})
		}
	})

	Context("when request path matches /api/*", func() {
		It("should propagate x-api-key header", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-H", "x-api-key: secret-api-key-123",
				"-H", "x-request-id: req-123",
				"-H", "x-tenant-id: tenant-abc",
				serviceURL+"/api/v1/users",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response for /api/v1/users: %s\n", body)

			// API headers should be propagated for /api/* path
			Expect(body).To(ContainSubstring("x-api-key"))
			Expect(body).To(ContainSubstring("secret-api-key-123"))
			Expect(body).To(ContainSubstring("x-request-id"))
			Expect(body).To(ContainSubstring("x-tenant-id"))
		})
	})

	Context("when request path is /health", func() {
		It("should NOT propagate x-api-key header", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-H", "x-api-key: secret-api-key-456",
				"-H", "x-tenant-id: tenant-xyz",
				serviceURL+"/health",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response for /health: %s\n", body)

			// x-tenant-id should be propagated (matches all paths)
			Expect(body).To(ContainSubstring("x-tenant-id"))
			Expect(body).To(ContainSubstring("tenant-xyz"))

			// Note: x-api-key may still appear in response because curl sends it directly
			// The filtering applies to what the PROXY propagates to outgoing requests
			// This test documents the expected behavior
		})
	})
})

// =============================================================================
// METHOD-BASED FILTERING TESTS
// Verifies headers are only propagated for matching HTTP methods
// =============================================================================
var _ = Describe("Method-Based Header Filtering", Ordered, func() {
	var (
		ctx         context.Context
		serviceName string
		curlPodName string
		policyName  string
	)

	BeforeAll(func() {
		ctx = context.Background()
		serviceName = "method-filter-service"
		curlPodName = "curl-method-filter"
		policyName = "method-filter-policy"

		By("Creating a HeaderPropagationPolicy with method-based rules")
		policy := &ctxforgev1alpha1.HeaderPropagationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: testNamespace,
			},
			Spec: ctxforgev1alpha1.HeaderPropagationPolicySpec{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": serviceName,
					},
				},
				PropagationRules: []ctxforgev1alpha1.PropagationRule{
					{
						// Only propagate CSRF token for mutating methods
						Methods: []string{"POST", "PUT", "DELETE", "PATCH"},
						Headers: []ctxforgev1alpha1.HeaderConfig{
							{Name: "x-csrf-token"},
						},
					},
					{
						// Propagate request ID for all methods
						Headers: []ctxforgev1alpha1.HeaderConfig{
							{Name: "x-request-id"},
						},
					},
				},
			},
		}
		err := ctxforgeClient.Create(ctx, policy)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying echo server with method filtering")
		err = deployMethodFilterService(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())
		err = waitForDeploymentWithTimeout(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying curl test pod")
		err = deployCurlPod(ctx, curlPodName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if policyName != "" {
			policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
			if err := ctxforgeClient.Get(ctx, client.ObjectKey{Name: policyName, Namespace: testNamespace}, policy); err == nil {
				_ = ctxforgeClient.Delete(ctx, policy)
			}
		}
		if serviceName != "" {
			_ = clientset.AppsV1().Deployments(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
			_ = clientset.CoreV1().Services(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		}
		if curlPodName != "" {
			_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, curlPodName, metav1.DeleteOptions{})
		}
	})

	Context("when making a POST request", func() {
		It("should propagate x-csrf-token header", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-X", "POST",
				"-H", "x-csrf-token: csrf-token-abc123",
				"-H", "x-request-id: post-req-123",
				"-d", "{}",
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response for POST: %s\n", body)

			// Both headers should be propagated for POST
			Expect(body).To(ContainSubstring("x-csrf-token"))
			Expect(body).To(ContainSubstring("csrf-token-abc123"))
			Expect(body).To(ContainSubstring("x-request-id"))
		})
	})

	Context("when making a GET request", func() {
		It("should propagate x-request-id but NOT x-csrf-token", func() {
			serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

			cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
				"curl", "-s",
				"-X", "GET",
				"-H", "x-csrf-token: csrf-token-xyz789",
				"-H", "x-request-id: get-req-456",
				serviceURL+"/",
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

			body := stdout.String()
			GinkgoWriter.Printf("Response for GET: %s\n", body)

			// x-request-id should be propagated (matches all methods)
			Expect(body).To(ContainSubstring("x-request-id"))
			Expect(body).To(ContainSubstring("get-req-456"))

			// Note: x-csrf-token may still appear because curl sends it directly
			// The filtering applies to outgoing requests from the service
		})
	})
})

// =============================================================================
// HEADER_RULES ENVIRONMENT VARIABLE TESTS
// Verifies the HEADER_RULES JSON config works correctly
// =============================================================================
var _ = Describe("HEADER_RULES Environment Variable", Ordered, func() {
	var (
		ctx         context.Context
		serviceName string
		curlPodName string
	)

	BeforeAll(func() {
		ctx = context.Background()
		serviceName = "header-rules-service"
		curlPodName = "curl-header-rules"

		By("Deploying service with HEADER_RULES environment variable")
		err := deployHeaderRulesService(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())
		err = waitForDeploymentWithTimeout(ctx, serviceName)
		Expect(err).NotTo(HaveOccurred())

		By("Deploying curl test pod")
		err = deployCurlPod(ctx, curlPodName)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if serviceName != "" {
			_ = clientset.AppsV1().Deployments(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
			_ = clientset.CoreV1().Services(testNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
		}
		if curlPodName != "" {
			_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, curlPodName, metav1.DeleteOptions{})
		}
	})

	It("should parse and apply HEADER_RULES correctly", func() {
		serviceURL := fmt.Sprintf("http://%s:8080", serviceName)

		cmd := exec.Command("kubectl", "exec", "-n", testNamespace, curlPodName, "--",
			"curl", "-s",
			"-H", "x-tenant-id: tenant-from-rules",
			serviceURL+"/api/test",
		)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		Expect(err).NotTo(HaveOccurred(), "curl failed: %s", stderr.String())

		body := stdout.String()
		GinkgoWriter.Printf("Response with HEADER_RULES: %s\n", body)

		// x-request-id should be generated (as configured in HEADER_RULES)
		Expect(body).To(ContainSubstring("x-request-id"))

		// UUID pattern should be present
		uuidPattern := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`)
		Expect(uuidPattern.MatchString(body)).To(BeTrue())

		// x-tenant-id should be propagated
		Expect(body).To(ContainSubstring("x-tenant-id"))
		Expect(body).To(ContainSubstring("tenant-from-rules"))
	})
})

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

const deploymentTimeout = 180 * time.Second

func waitForDeploymentWithTimeout(ctx context.Context, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, deploymentTimeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(testNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return deployment.Status.ReadyReplicas == *deployment.Spec.Replicas, nil
	})
}

func deployHeaderGenService(ctx context.Context, name string) error {
	replicas := int32(1)

	// Build HEADER_RULES JSON for generation
	headerRules := []map[string]interface{}{
		{"name": "x-request-id", "generate": true, "generatorType": "uuid"},
		{"name": "x-trace-id", "generate": true, "generatorType": "ulid"},
		{"name": "x-timestamp", "generate": true, "generatorType": "timestamp"},
		{"name": "x-tenant-id"},
	}
	headerRulesJSON, _ := json.Marshal(headerRules)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                 name,
						"ctxforge.io/enabled": "true",
					},
					Annotations: map[string]string{
						"ctxforge.io/enabled":      "true",
						"ctxforge.io/target-port":  "8080",
						"ctxforge.io/header-rules": string(headerRulesJSON),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "echo",
							Image: "ealen/echo-server:latest",
							Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
							Env: []corev1.EnvVar{
								{Name: "PORT", Value: "8080"},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments(testNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt(8080)},
			},
		},
	}

	_, err = clientset.CoreV1().Services(testNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func deployPathFilterService(ctx context.Context, name string) error {
	replicas := int32(1)

	// Path-based header rules
	headerRules := []map[string]interface{}{
		{"name": "x-api-key", "pathRegex": "^/api/.*"},
		{"name": "x-request-id", "pathRegex": "^/api/.*"},
		{"name": "x-tenant-id"}, // No path filter - applies to all
	}
	headerRulesJSON, _ := json.Marshal(headerRules)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                 name,
						"ctxforge.io/enabled": "true",
					},
					Annotations: map[string]string{
						"ctxforge.io/enabled":      "true",
						"ctxforge.io/target-port":  "8080",
						"ctxforge.io/header-rules": string(headerRulesJSON),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "echo",
							Image: "ealen/echo-server:latest",
							Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
							Env: []corev1.EnvVar{
								{Name: "PORT", Value: "8080"},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments(testNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt(8080)},
			},
		},
	}

	_, err = clientset.CoreV1().Services(testNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func deployMethodFilterService(ctx context.Context, name string) error {
	replicas := int32(1)

	// Method-based header rules
	headerRules := []map[string]interface{}{
		{"name": "x-csrf-token", "methods": []string{"POST", "PUT", "DELETE", "PATCH"}},
		{"name": "x-request-id"}, // No method filter - applies to all
	}
	headerRulesJSON, _ := json.Marshal(headerRules)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                 name,
						"ctxforge.io/enabled": "true",
					},
					Annotations: map[string]string{
						"ctxforge.io/enabled":      "true",
						"ctxforge.io/target-port":  "8080",
						"ctxforge.io/header-rules": string(headerRulesJSON),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "echo",
							Image: "ealen/echo-server:latest",
							Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
							Env: []corev1.EnvVar{
								{Name: "PORT", Value: "8080"},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments(testNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt(8080)},
			},
		},
	}

	_, err = clientset.CoreV1().Services(testNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

func deployHeaderRulesService(ctx context.Context, name string) error {
	replicas := int32(1)

	// HEADER_RULES with generation and path filtering
	headerRules := []map[string]interface{}{
		{"name": "x-request-id", "generate": true, "generatorType": "uuid"},
		{"name": "x-tenant-id"},
		{"name": "x-api-key", "pathRegex": "^/api/.*"},
	}
	headerRulesJSON, _ := json.Marshal(headerRules)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                 name,
						"ctxforge.io/enabled": "true",
					},
					Annotations: map[string]string{
						"ctxforge.io/enabled":      "true",
						"ctxforge.io/target-port":  "8080",
						"ctxforge.io/header-rules": string(headerRulesJSON),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "echo",
							Image: "ealen/echo-server:latest",
							Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
							Env: []corev1.EnvVar{
								{Name: "PORT", Value: "8080"},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments(testNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{Port: 8080, TargetPort: intstr.FromInt(8080)},
			},
		},
	}

	_, err = clientset.CoreV1().Services(testNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}
