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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodIPAnnotationKey = "example.com/pod-ip"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := req.String()
	klog.Infof("Reconcile pod: %s", key)
	start := time.Now()
	defer func() {
		klog.Infof("Finished reconcile pod[%s]: %v", key, time.Since(start))
	}()

	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 3 * time.Second}, err
	}

	// Pod has no IP yet, skip
	if pod.Status.PodIP == "" {
		klog.V(4).Infof("Pod %s has no IP yet, skip", key)
		return ctrl.Result{}, nil
	}

	// Check if annotation already exists with correct value
	if pod.Annotations != nil {
		if existingIP, ok := pod.Annotations[PodIPAnnotationKey]; ok && existingIP == pod.Status.PodIP {
			klog.V(4).Infof("Pod %s already has correct IP annotation", key)
			return ctrl.Result{}, nil
		}
	}

	// Add or update the annotation
	patch := client.MergeFrom(pod.DeepCopy())
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[PodIPAnnotationKey] = pod.Status.PodIP

	if err := r.Patch(ctx, pod, patch); err != nil {
		klog.Errorf("Failed to patch pod %s annotation: %v", key, err)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, err
	}

	klog.Infof("Added IP annotation %s=%s to pod %s", PodIPAnnotationKey, pod.Status.PodIP, key)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(&podIPPredicate{})).
		Named("pod").
		Complete(r)
}