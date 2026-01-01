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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctxforgev1alpha1 "github.com/bgruszka/contextforge/api/v1alpha1"
)

const (
	// ConditionTypeReady indicates whether the policy is ready and applied
	ConditionTypeReady = "Ready"

	// RequeueAfterNoMatches is the requeue interval when no pods match the selector.
	// We check periodically in case pods are created outside of our watch events.
	RequeueAfterNoMatches = 30 * time.Second

	// RequeueAfterPendingPods is the requeue interval when pods exist but aren't ready.
	// Shorter interval to quickly detect when pods become ready.
	RequeueAfterPendingPods = 10 * time.Second
)

// HeaderPropagationPolicyReconciler reconciles a HeaderPropagationPolicy object
type HeaderPropagationPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ctxforge.ctxforge.io,resources=headerpropagationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ctxforge.ctxforge.io,resources=headerpropagationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ctxforge.ctxforge.io,resources=headerpropagationpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// The controller performs the following actions:
// 1. Fetches the HeaderPropagationPolicy resource
// 2. Lists pods matching the policy's PodSelector in the same namespace
// 3. Updates the status with the count of matched pods
// 4. Sets the Ready condition based on whether pods are found
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *HeaderPropagationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the HeaderPropagationPolicy instance
	policy := &ctxforgev1alpha1.HeaderPropagationPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			// Policy was deleted, nothing to do
			log.Info("HeaderPropagationPolicy resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch HeaderPropagationPolicy")
		return ctrl.Result{}, err
	}

	// Build label selector from PodSelector
	var selector labels.Selector
	var err error
	if policy.Spec.PodSelector != nil {
		selector, err = metav1.LabelSelectorAsSelector(policy.Spec.PodSelector)
		if err != nil {
			log.Error(err, "Failed to parse PodSelector")
			r.setReadyCondition(ctx, policy, metav1.ConditionFalse, "InvalidSelector", "Failed to parse PodSelector: "+err.Error())
			return ctrl.Result{}, err
		}
	} else {
		// Empty selector matches all pods in namespace
		selector = labels.Everything()
	}

	// List pods matching the selector in the same namespace
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(policy.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}
	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods")
		r.setReadyCondition(ctx, policy, metav1.ConditionFalse, "ListPodsFailed", "Failed to list pods: "+err.Error())
		return ctrl.Result{}, err
	}

	// Count pods by state
	var matchedPods int32
	var pendingPods int32
	var totalSelectorMatches int32

	for _, pod := range podList.Items {
		// Check if the pod has the ctxforge sidecar
		hasSidecar := false
		for _, container := range pod.Spec.Containers {
			if container.Name == "ctxforge-proxy" {
				hasSidecar = true
				break
			}
		}

		if hasSidecar {
			totalSelectorMatches++
			switch pod.Status.Phase {
			case corev1.PodRunning:
				matchedPods++
			case corev1.PodPending:
				pendingPods++
			}
		}
	}

	// Determine if status changed
	statusChanged := policy.Status.AppliedToPods != matchedPods ||
		policy.Status.ObservedGeneration != policy.Generation

	// Update status
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.AppliedToPods = matchedPods

	// Set Ready condition
	if matchedPods > 0 {
		r.setReadyCondition(ctx, policy, metav1.ConditionTrue, "PolicyApplied",
			"Policy is applied to pods with contextforge-proxy sidecar")
	} else {
		r.setReadyCondition(ctx, policy, metav1.ConditionFalse, "NoMatchingPods",
			"No running pods with contextforge-proxy sidecar match the selector")
	}

	// Update the status
	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, "Failed to update HeaderPropagationPolicy status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciled HeaderPropagationPolicy",
		"appliedToPods", matchedPods,
		"pendingPods", pendingPods,
		"totalWithSidecar", totalSelectorMatches,
		"selector", selector.String(),
		"statusChanged", statusChanged)

	// Determine requeue strategy based on current state
	//
	// Event-driven reconciliation handles most changes (pod create/delete/update),
	// but periodic requeue is needed for edge cases:
	// 1. Pod phase transitions (Pending -> Running) may not trigger events
	// 2. Pods created in other namespaces that match a cluster-wide policy
	// 3. Recovery from controller restarts
	//
	// Optimization: Only requeue when there's a reason to check again
	if pendingPods > 0 {
		// Pods are starting up, check again soon
		return ctrl.Result{RequeueAfter: RequeueAfterPendingPods}, nil
	}

	if matchedPods == 0 && totalSelectorMatches == 0 {
		// No matching pods at all - periodic check as fallback
		// This catches pods created outside our watch events
		return ctrl.Result{RequeueAfter: RequeueAfterNoMatches}, nil
	}

	// Pods are running and stable - rely on event-driven reconciliation
	// No periodic requeue needed; pod events will trigger reconciliation
	return ctrl.Result{}, nil
}

// setReadyCondition sets the Ready condition on the policy
func (r *HeaderPropagationPolicyReconciler) setReadyCondition(_ context.Context, policy *ctxforgev1alpha1.HeaderPropagationPolicy, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             status,
		ObservedGeneration: policy.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&policy.Status.Conditions, condition)
}

// findPoliciesForPod returns a list of reconcile requests for all policies
// that might apply to the given pod based on namespace matching.
// This enables the controller to react when pods are created, updated, or deleted.
func (r *HeaderPropagationPolicyReconciler) findPoliciesForPod(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	// List all policies in the pod's namespace
	policyList := &ctxforgev1alpha1.HeaderPropagationPolicyList{}
	if err := r.List(ctx, policyList, client.InNamespace(pod.Namespace)); err != nil {
		log.Error(err, "Failed to list HeaderPropagationPolicies for pod", "pod", pod.Name)
		return nil
	}

	// Build reconcile requests for policies whose selector matches this pod
	var requests []reconcile.Request
	for _, policy := range policyList.Items {
		var selector labels.Selector
		var err error
		if policy.Spec.PodSelector != nil {
			selector, err = metav1.LabelSelectorAsSelector(policy.Spec.PodSelector)
			if err != nil {
				continue
			}
		} else {
			selector = labels.Everything()
		}

		if selector.Matches(labels.Set(pod.Labels)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&policy),
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *HeaderPropagationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ctxforgev1alpha1.HeaderPropagationPolicy{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForPod),
		).
		Named("headerpropagationpolicy").
		Complete(r)
}
