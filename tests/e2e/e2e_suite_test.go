package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctxforgev1alpha1 "github.com/bgruszka/contextforge/api/v1alpha1"
)

var (
	clientset      *kubernetes.Clientset
	ctxforgeClient client.Client
	testNamespace  string
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ContextForge E2E Suite")
}

var _ = BeforeSuite(func() {
	var err error

	// Use KUBECONFIG or default location
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).NotTo(HaveOccurred(), "Failed to build kubeconfig")

	clientset, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes client")

	// Create controller-runtime client for CRDs
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(ctxforgev1alpha1.AddToScheme(scheme)).To(Succeed())
	ctxforgeClient, err = client.New(config, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "Failed to create controller-runtime client")

	// Create test namespace
	testNamespace = fmt.Sprintf("ctxforge-e2e-%d", time.Now().Unix())
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
			Labels: map[string]string{
				"ctxforge.io/injection": "enabled",
			},
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

	// Wait for namespace to be ready
	err = wait.PollUntilContextTimeout(context.Background(), time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		ns, err := clientset.CoreV1().Namespaces().Get(ctx, testNamespace, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return ns.Status.Phase == corev1.NamespaceActive, nil
	})
	Expect(err).NotTo(HaveOccurred(), "Namespace did not become ready")
})

var _ = AfterSuite(func() {
	if clientset != nil && testNamespace != "" {
		// Clean up test namespace
		err := clientset.CoreV1().Namespaces().Delete(context.Background(), testNamespace, metav1.DeleteOptions{})
		if err != nil {
			GinkgoWriter.Printf("Warning: failed to delete test namespace: %v\n", err)
		}
	}
})
