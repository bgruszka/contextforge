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

package v1

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// AnnotationEnabled is the annotation key to enable sidecar injection
	AnnotationEnabled = "ctxforge.io/enabled"
	// AnnotationHeaders is the annotation key for headers to propagate
	AnnotationHeaders = "ctxforge.io/headers"
	// AnnotationTargetPort is the annotation key for the target application port
	AnnotationTargetPort = "ctxforge.io/target-port"
	// AnnotationInjected marks a pod as already injected
	AnnotationInjected = "ctxforge.io/injected"

	// ProxyContainerName is the name of the injected sidecar container
	ProxyContainerName = "ctxforge-proxy"
	// DefaultProxyImage is the default image for the proxy sidecar
	DefaultProxyImage = "ghcr.io/bgruszka/contextforge-proxy:latest"
	// DefaultTargetPort is the default port of the application container
	DefaultTargetPort = "8080"
	// ProxyPort is the port the proxy listens on
	ProxyPort = 9090
)

var podlog = logf.Log.WithName("pod-webhook")

// SetupPodWebhookWithManager registers the webhook for Pod in the manager.
func SetupPodWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&corev1.Pod{}).
		WithValidator(&PodCustomValidator{}).
		WithDefaulter(&PodCustomDefaulter{
			ProxyImage: getEnvOrDefault("PROXY_IMAGE", DefaultProxyImage),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create,versions=v1,name=mpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomDefaulter handles sidecar injection for pods
type PodCustomDefaulter struct {
	ProxyImage string
}

var _ webhook.CustomDefaulter = &PodCustomDefaulter{}

// Default implements webhook.CustomDefaulter to inject the sidecar
func (d *PodCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod object but got %T", obj)
	}

	if !d.shouldInject(pod) {
		return nil
	}

	headers := d.extractHeaders(pod)
	if len(headers) == 0 {
		podlog.Info("Skipping injection: no headers specified", "pod", pod.Name)
		return nil
	}

	if d.isAlreadyInjected(pod) {
		podlog.Info("Skipping injection: already injected", "pod", pod.Name)
		return nil
	}

	podlog.Info("Injecting sidecar", "pod", pod.Name, "headers", headers)

	if err := d.injectSidecar(pod, headers); err != nil {
		return fmt.Errorf("failed to inject sidecar: %w", err)
	}

	d.modifyAppContainers(pod)
	d.markAsInjected(pod)

	return nil
}

// shouldInject checks if the pod should have sidecar injection
func (d *PodCustomDefaulter) shouldInject(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	enabled, ok := pod.Annotations[AnnotationEnabled]
	return ok && enabled == "true"
}

// extractHeaders parses the headers annotation
func (d *PodCustomDefaulter) extractHeaders(pod *corev1.Pod) []string {
	if pod.Annotations == nil {
		return nil
	}
	headersStr, ok := pod.Annotations[AnnotationHeaders]
	if !ok || headersStr == "" {
		return nil
	}

	parts := strings.Split(headersStr, ",")
	headers := make([]string, 0, len(parts))
	for _, part := range parts {
		header := strings.TrimSpace(part)
		if header != "" {
			headers = append(headers, header)
		}
	}
	return headers
}

// isAlreadyInjected checks if the sidecar is already present
func (d *PodCustomDefaulter) isAlreadyInjected(pod *corev1.Pod) bool {
	if pod.Annotations != nil {
		if _, ok := pod.Annotations[AnnotationInjected]; ok {
			return true
		}
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == ProxyContainerName {
			return true
		}
	}
	return false
}

// injectSidecar adds the proxy container to the pod
func (d *PodCustomDefaulter) injectSidecar(pod *corev1.Pod, headers []string) error {
	targetPort := DefaultTargetPort
	if pod.Annotations != nil {
		if port, ok := pod.Annotations[AnnotationTargetPort]; ok && port != "" {
			targetPort = port
		}
	}

	sidecar := corev1.Container{
		Name:            ProxyContainerName,
		Image:           d.ProxyImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: ProxyPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "HEADERS_TO_PROPAGATE",
				Value: strings.Join(headers, ","),
			},
			{
				Name:  "TARGET_HOST",
				Value: fmt.Sprintf("localhost:%s", targetPort),
			},
			{
				Name:  "PROXY_PORT",
				Value: fmt.Sprintf("%d", ProxyPort),
			},
			{
				Name:  "LOG_LEVEL",
				Value: "info",
			},
			{
				Name:  "LOG_FORMAT",
				Value: "json",
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("10Mi"),
				corev1.ResourceCPU:    resource.MustParse("10m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("50Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                int64Ptr(65532),
			AllowPrivilegeEscalation: boolPtr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			ReadOnlyRootFilesystem: boolPtr(true),
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(ProxyPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ready",
					Port: intstr.FromInt(ProxyPort),
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       5,
		},
	}

	pod.Spec.Containers = append(pod.Spec.Containers, sidecar)
	return nil
}

// modifyAppContainers adds HTTP_PROXY env vars to application containers
func (d *PodCustomDefaulter) modifyAppContainers(pod *corev1.Pod) {
	proxyEnvVars := []corev1.EnvVar{
		{
			Name:  "HTTP_PROXY",
			Value: fmt.Sprintf("http://localhost:%d", ProxyPort),
		},
		{
			Name:  "HTTPS_PROXY",
			Value: fmt.Sprintf("http://localhost:%d", ProxyPort),
		},
		{
			Name:  "NO_PROXY",
			Value: "localhost,127.0.0.1",
		},
	}

	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == ProxyContainerName {
			continue
		}
		pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, proxyEnvVars...)
	}
}

// markAsInjected adds an annotation to indicate the pod was injected
func (d *PodCustomDefaulter) markAsInjected(pod *corev1.Pod) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[AnnotationInjected] = "true"
}

// +kubebuilder:webhook:path=/validate--v1-pod,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=vpod-v1.kb.io,admissionReviewVersions=v1

// PodCustomValidator validates Pod resources
type PodCustomValidator struct{}

var _ webhook.CustomValidator = &PodCustomValidator{}

// ValidateCreate validates pod creation
func (v *PodCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("expected a Pod object but got %T", obj)
	}

	if pod.Annotations == nil {
		return nil, nil
	}

	if enabled, ok := pod.Annotations[AnnotationEnabled]; ok && enabled == "true" {
		headers, hasHeaders := pod.Annotations[AnnotationHeaders]
		if !hasHeaders || strings.TrimSpace(headers) == "" {
			return admission.Warnings{
				"ctxforge.io/enabled is set but no headers specified in ctxforge.io/headers",
			}, nil
		}
	}

	return nil, nil
}

// ValidateUpdate validates pod updates
func (v *PodCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return nil, fmt.Errorf("expected a Pod object but got %T", newObj)
	}
	_ = pod
	return nil, nil
}

// ValidateDelete validates pod deletion
func (v *PodCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
