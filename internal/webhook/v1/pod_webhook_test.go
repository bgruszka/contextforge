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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodCustomDefaulter_ShouldInject(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "enabled annotation true",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationEnabled: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "enabled annotation false",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationEnabled: "false",
					},
				},
			},
			expected: false,
		},
		{
			name:     "no annotations",
			pod:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "missing enabled annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"other": "value",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaulter.shouldInject(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPodCustomDefaulter_ExtractHeaders(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected []string
	}{
		{
			name: "single header",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHeaders: "x-request-id",
					},
				},
			},
			expected: []string{"x-request-id"},
		},
		{
			name: "multiple headers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHeaders: "x-request-id,x-dev-id,x-tenant-id",
					},
				},
			},
			expected: []string{"x-request-id", "x-dev-id", "x-tenant-id"},
		},
		{
			name: "headers with spaces",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHeaders: "x-request-id , x-dev-id , x-tenant-id",
					},
				},
			},
			expected: []string{"x-request-id", "x-dev-id", "x-tenant-id"},
		},
		{
			name:     "no annotations",
			pod:      &corev1.Pod{},
			expected: nil,
		},
		{
			name: "empty headers annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHeaders: "",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaulter.extractHeaders(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPodCustomDefaulter_IsAlreadyInjected(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "has injected annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationInjected: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "has sidecar container",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
						{Name: ProxyContainerName},
					},
				},
			},
			expected: true,
		},
		{
			name: "not injected",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := defaulter.isAlreadyInjected(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPodCustomDefaulter_InjectSidecar(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: "test-image:v1"}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "myapp:latest"},
			},
		},
	}

	headers := []string{"x-request-id", "x-dev-id"}
	defaulter.injectSidecar(pod, headers)

	assert.Len(t, pod.Spec.Containers, 2)

	var sidecar *corev1.Container
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == ProxyContainerName {
			sidecar = &pod.Spec.Containers[i]
			break
		}
	}

	require.NotNil(t, sidecar, "Sidecar container should be present")
	assert.Equal(t, "test-image:v1", sidecar.Image)
	assert.Equal(t, corev1.PullIfNotPresent, sidecar.ImagePullPolicy)

	var headersEnv *corev1.EnvVar
	for i := range sidecar.Env {
		if sidecar.Env[i].Name == "HEADERS_TO_PROPAGATE" {
			headersEnv = &sidecar.Env[i]
			break
		}
	}
	require.NotNil(t, headersEnv)
	assert.Equal(t, "x-request-id,x-dev-id", headersEnv.Value)

	assert.NotNil(t, sidecar.SecurityContext)
	assert.True(t, *sidecar.SecurityContext.RunAsNonRoot)
	assert.Equal(t, int64(65532), *sidecar.SecurityContext.RunAsUser)

	assert.NotNil(t, sidecar.LivenessProbe)
	assert.NotNil(t, sidecar.ReadinessProbe)
}

func TestPodCustomDefaulter_InjectSidecar_CustomTargetPort(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				AnnotationTargetPort: "3000",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	defaulter.injectSidecar(pod, []string{"x-request-id"})

	var sidecar *corev1.Container
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == ProxyContainerName {
			sidecar = &pod.Spec.Containers[i]
			break
		}
	}
	require.NotNil(t, sidecar)

	var targetHostEnv *corev1.EnvVar
	for i := range sidecar.Env {
		if sidecar.Env[i].Name == "TARGET_HOST" {
			targetHostEnv = &sidecar.Env[i]
			break
		}
	}
	require.NotNil(t, targetHostEnv)
	assert.Equal(t, "localhost:3000", targetHostEnv.Value)
}

func TestPodCustomDefaulter_ModifyAppContainers(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app1"},
				{Name: "app2"},
				{Name: ProxyContainerName},
			},
		},
	}

	defaulter.modifyAppContainers(pod)

	for _, container := range pod.Spec.Containers {
		if container.Name == ProxyContainerName {
			assert.Empty(t, container.Env, "Proxy container should not have proxy env vars")
			continue
		}

		var httpProxy, noProxy bool
		var hasHTTPSProxy bool
		for _, env := range container.Env {
			switch env.Name {
			case "HTTP_PROXY":
				httpProxy = true
				assert.Equal(t, "http://localhost:9090", env.Value)
			case "HTTPS_PROXY":
				hasHTTPSProxy = true
			case "NO_PROXY":
				noProxy = true
				assert.Equal(t, "localhost,127.0.0.1", env.Value)
			}
		}

		assert.True(t, httpProxy, "HTTP_PROXY should be set for %s", container.Name)
		assert.False(t, hasHTTPSProxy, "HTTPS_PROXY should NOT be set (proxy only handles HTTP)")
		assert.True(t, noProxy, "NO_PROXY should be set for %s", container.Name)
	}
}

func TestPodCustomDefaulter_Default_FullInjection(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: "test-proxy:v1"}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				AnnotationEnabled: "true",
				AnnotationHeaders: "x-request-id,x-tenant-id",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "myapp:latest"},
			},
		},
	}

	err := defaulter.Default(context.Background(), pod)

	require.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 2)
	assert.Equal(t, "true", pod.Annotations[AnnotationInjected])

	var foundProxy bool
	for _, c := range pod.Spec.Containers {
		if c.Name == ProxyContainerName {
			foundProxy = true
			break
		}
	}
	assert.True(t, foundProxy)
}

func TestPodCustomDefaulter_Default_SkipsWhenNotEnabled(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	err := defaulter.Default(context.Background(), pod)

	require.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 1)
}

func TestPodCustomDefaulter_Default_SkipsWhenAlreadyInjected(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				AnnotationEnabled:  "true",
				AnnotationHeaders:  "x-request-id",
				AnnotationInjected: "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	err := defaulter.Default(context.Background(), pod)

	require.NoError(t, err)
	assert.Len(t, pod.Spec.Containers, 1)
}

func TestValidateTargetPort(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid port",
			port:        "8080",
			expectError: false,
		},
		{
			name:        "valid port min",
			port:        "1",
			expectError: false,
		},
		{
			name:        "valid port max",
			port:        "65535",
			expectError: false,
		},
		{
			name:        "non-numeric port",
			port:        "abc",
			expectError: true,
			errorMsg:    "must be a number",
		},
		{
			name:        "port too low",
			port:        "0",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "port too high",
			port:        "65536",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "negative port",
			port:        "-1",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "port equals proxy port",
			port:        "9090",
			expectError: true,
			errorMsg:    "cannot be the same as proxy port",
		},
		{
			name:        "empty port",
			port:        "",
			expectError: true,
			errorMsg:    "must be a number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTargetPort(tt.port)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPodCustomDefaulter_InjectSidecar_InvalidTargetPort(t *testing.T) {
	defaulter := &PodCustomDefaulter{ProxyImage: DefaultProxyImage}

	tests := []struct {
		name         string
		port         string
		expectedPort string
	}{
		{
			name:         "non-numeric uses default",
			port:         "abc",
			expectedPort: DefaultTargetPort,
		},
		{
			name:         "out of range uses default",
			port:         "99999",
			expectedPort: DefaultTargetPort,
		},
		{
			name:         "proxy port conflict uses default",
			port:         "9090",
			expectedPort: DefaultTargetPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						AnnotationTargetPort: tt.port,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			}

			defaulter.injectSidecar(pod, []string{"x-request-id"})

			var sidecar *corev1.Container
			for i := range pod.Spec.Containers {
				if pod.Spec.Containers[i].Name == ProxyContainerName {
					sidecar = &pod.Spec.Containers[i]
					break
				}
			}
			require.NotNil(t, sidecar)

			var targetHostEnv *corev1.EnvVar
			for i := range sidecar.Env {
				if sidecar.Env[i].Name == "TARGET_HOST" {
					targetHostEnv = &sidecar.Env[i]
					break
				}
			}
			require.NotNil(t, targetHostEnv)
			assert.Equal(t, "localhost:"+tt.expectedPort, targetHostEnv.Value)
		})
	}
}

func TestPodCustomValidator_ValidateCreate(t *testing.T) {
	validator := &PodCustomValidator{}

	tests := []struct {
		name         string
		pod          *corev1.Pod
		expectError  bool
		warnExpected bool
	}{
		{
			name: "valid pod with headers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationEnabled: "true",
						AnnotationHeaders: "x-request-id",
					},
				},
			},
			expectError:  false,
			warnExpected: false,
		},
		{
			name: "enabled without headers - warning",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationEnabled: "true",
					},
				},
			},
			expectError:  false,
			warnExpected: true,
		},
		{
			name:         "no annotations",
			pod:          &corev1.Pod{},
			expectError:  false,
			warnExpected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := validator.ValidateCreate(context.Background(), tt.pod)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.warnExpected {
				assert.NotEmpty(t, warnings)
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}
