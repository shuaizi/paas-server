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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	testv1 "kubeassemble.cn/guide/api/v1"
)

// GuideReconciler reconciles a Guide object
type GuideReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=test.kubeassemble.cn,resources=guides/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Guide object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *GuideReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	_ = logf.FromContext(ctx)

	key := req.String()
	klog.Infof("Reconcile guide: %s", key)
	start := time.Now()
	msg := ""
	defer func() {
		if err != nil {
			klog.Errorf("Failed to reconcile guide[%s] Error: %v", key, err)
		} else {
			if result.RequeueAfter > 0 {
				klog.Infof("reconcile guide[%s] need requeue: %v", key, time.Since(start))
			} else {
				klog.Infof("Finished reconcile guide[%s]: %v", key, time.Since(start))
			}
		}
	}()

	guide := &testv1.Guide{}
	if err = r.Get(context.TODO(), req.NamespacedName, guide); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}

	if guide.Spec.WorkloadName == nil || guide.Spec.Replica == nil {
		klog.Warningf("workload name or replica not exist")
		return reconcile.Result{}, nil
	}

	nsName := types.NamespacedName{
		Namespace: "default",
		Name:      *guide.Spec.WorkloadName,
	}

	sts := &appsv1.StatefulSet{}
	if err = r.Get(context.TODO(), nsName, sts); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{RequeueAfter: 3 * time.Second}, err
	}

	if *sts.Spec.Replicas == *guide.Spec.Replica {
		klog.Infof("same replica of guide[%s]", key)
	} else {
		sts.Spec.Replicas = guide.Spec.Replica
		if err = r.Update(context.TODO(), sts); err != nil {
			klog.Errorf("Failed to update statefulset %s/%s: %v", sts.Namespace, sts.Name, err)
			return reconcile.Result{RequeueAfter: 3 * time.Second}, err
		}
		msg = fmt.Sprintf("update sts %s/%s successfully", sts.Namespace, sts.Name)
		klog.Info(msg)
	}

	// update guide status
	guide.Status.LastTransitionTime = metav1.Time{
		Time: time.Now(),
	}
	guide.Status.Message = msg
	if err = r.Status().Update(ctx, guide); err != nil {
		klog.Errorf("Failed to update guide[%s] Status: %v", key, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuideReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testv1.Guide{}, builder.WithPredicates(&guidePredicate{})).
		Named("guide").
		Complete(r)
}
