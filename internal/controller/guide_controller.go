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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	testv1 "kubeassemble.cn/guide/api/v1"
)

const (
	// ConditionAvailable is the Guide condition that reports whether the
	// referenced StatefulSet is at the requested replica count.
	ConditionAvailable = "Available"

	ReasonInvalidSpec         = "InvalidSpec"
	ReasonStatefulSetNotFound = "StatefulSetNotFound"
	ReasonReplicasSynced      = "ReplicasSynced"

	requeueOnError    = 3 * time.Second
	requeueOnNotFound = 15 * time.Second
)

// GuideReconciler reconciles a Guide object
type GuideReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch

// Reconcile moves the StatefulSet named by spec.workloadName, in the Guide's
// own namespace, to spec.replica. A missing workload is reported on the Guide
// status and retried; an incomplete spec is reported and left until the spec changes.
func (r *GuideReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	key := req.String()
	klog.Infof("Reconcile guide: %s", key)
	start := time.Now()
	defer func() {
		if err != nil {
			klog.Errorf("Failed to reconcile guide[%s] Error: %v", key, err)
			return
		}
		if result.RequeueAfter > 0 {
			klog.Infof("reconcile guide[%s] need requeue: %v", key, time.Since(start))
			return
		}
		klog.Infof("Finished reconcile guide[%s]: %v", key, time.Since(start))
	}()

	guide := &testv1.Guide{}
	err = r.Get(ctx, req.NamespacedName, guide)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{RequeueAfter: requeueOnError}, err
	}

	workload, ok := workloadName(guide)
	if !ok || guide.Spec.Replica == nil {
		return r.finishStatus(ctx, req.NamespacedName, metav1.ConditionFalse, ReasonInvalidSpec,
			"spec.workloadName and spec.replica are required", 0)
	}

	stsKey := types.NamespacedName{Namespace: guide.Namespace, Name: workload}
	desired := *guide.Spec.Replica
	var updated bool
	updated, err = r.scaleStatefulSet(ctx, stsKey, desired)
	if errors.IsNotFound(err) {
		return r.finishStatus(ctx, req.NamespacedName, metav1.ConditionFalse, ReasonStatefulSetNotFound,
			fmt.Sprintf("statefulset %s/%s not found", stsKey.Namespace, stsKey.Name), requeueOnNotFound)
	}
	if err != nil {
		return reconcile.Result{RequeueAfter: requeueOnError}, err
	}

	var message string
	if updated {
		message = fmt.Sprintf("updated statefulset %s/%s replicas to %d", stsKey.Namespace, stsKey.Name, desired)
		klog.Info(message)
	} else {
		message = fmt.Sprintf("statefulset %s/%s already has %d replicas", stsKey.Namespace, stsKey.Name, desired)
		klog.Infof("same replica of guide[%s]", key)
	}
	return r.finishStatus(ctx, req.NamespacedName, metav1.ConditionTrue, ReasonReplicasSynced, message, 0)
}

func (r *GuideReconciler) finishStatus(ctx context.Context, key types.NamespacedName, status metav1.ConditionStatus, reason, message string, requeue time.Duration) (ctrl.Result, error) {
	if err := r.updateStatus(ctx, key, status, reason, message); err != nil {
		return ctrl.Result{RequeueAfter: requeueOnError}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *GuideReconciler) scaleStatefulSet(ctx context.Context, key types.NamespacedName, desired int32) (bool, error) {
	updated := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		updated = false
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, key, sts); err != nil {
			return err
		}
		if replicaCount(sts.Spec.Replicas) == desired {
			return nil
		}
		base := sts.DeepCopy()
		replicas := desired
		sts.Spec.Replicas = &replicas
		if err := r.Patch(ctx, sts, client.MergeFrom(base)); err != nil {
			return err
		}
		updated = true
		return nil
	})
	return updated, err
}

func (r *GuideReconciler) updateStatus(ctx context.Context, key types.NamespacedName, status metav1.ConditionStatus, reason, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		guide := &testv1.Guide{}
		if err := r.Get(ctx, key, guide); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		changed := meta.SetStatusCondition(&guide.Status.Conditions, metav1.Condition{
			Type:               ConditionAvailable,
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: guide.Generation,
		})
		if !changed && guide.Status.Message == message {
			return nil
		}
		guide.Status.Message = message
		guide.Status.LastTransitionTime = metav1.Now()
		return r.Status().Update(ctx, guide)
	})
}

// mapStatefulSetToGuides enqueues Guides in the StatefulSet's namespace whose
// spec.workloadName points at that StatefulSet. A replica change made outside
// the Guide is therefore reconciled again.
func (r *GuideReconciler) mapStatefulSetToGuides(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &testv1.GuideList{}
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		klog.Errorf("Failed to list guides in %s for statefulset %s: %v", obj.GetNamespace(), obj.GetName(), err)
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		guide := &list.Items[i]
		if guide.Spec.WorkloadName != nil && *guide.Spec.WorkloadName == obj.GetName() {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: guide.Namespace,
				Name:      guide.Name,
			}})
		}
	}
	return reqs
}

func workloadName(guide *testv1.Guide) (string, bool) {
	if guide.Spec.WorkloadName == nil || *guide.Spec.WorkloadName == "" {
		return "", false
	}
	return *guide.Spec.WorkloadName, true
}

// replicaCount treats a nil StatefulSet replica field as 1, matching the API default.
func replicaCount(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuideReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testv1.Guide{}, builder.WithPredicates(&guidePredicate{})).
		Watches(&appsv1.StatefulSet{},
			handler.EnqueueRequestsFromMapFunc(r.mapStatefulSetToGuides),
			builder.WithPredicates(&statefulSetReplicaPredicate{})).
		Named("guide").
		Complete(r)
}
