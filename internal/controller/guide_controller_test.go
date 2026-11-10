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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	testv1 "kubeassemble.cn/guide/api/v1"
)

const testWorkloadName = "web"

var _ = Describe("Guide Controller", func() {
	ctx := context.Background()
	var reconciler *GuideReconciler

	BeforeEach(func() {
		reconciler = &GuideReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	It("records an invalid spec and does not requeue", func() {
		ns := createNamespace(ctx)
		guide := createGuide(ctx, ns, "incomplete", nil, nil)

		result, err := reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		expectCondition(ctx, guide, metav1.ConditionFalse, ReasonInvalidSpec)
	})

	It("reports a missing StatefulSet in the Guide namespace and retries", func() {
		ns := createNamespace(ctx)
		name := "missing"
		replicas := int32(2)
		guide := createGuide(ctx, ns, "wants-missing", &name, &replicas)

		result, err := reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueOnNotFound))
		expectCondition(ctx, guide, metav1.ConditionFalse, ReasonStatefulSetNotFound)
	})

	It("scales the StatefulSet in the Guide namespace", func() {
		ns := createNamespace(ctx)
		name := testWorkloadName
		createStatefulSet(ctx, ns, name, int32Ptr(1))
		replicas := int32(3)
		guide := createGuide(ctx, ns, "scaler", &name, &replicas)

		result, err := reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts)).To(Succeed())
		Expect(sts.Spec.Replicas).NotTo(BeNil())
		Expect(*sts.Spec.Replicas).To(Equal(replicas))
		expectCondition(ctx, guide, metav1.ConditionTrue, ReasonReplicasSynced)

		result, err = reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts)).To(Succeed())
		Expect(*sts.Spec.Replicas).To(Equal(replicas))
	})

	It("does not write when the defaulted replica count already matches", func() {
		ns := createNamespace(ctx)
		name := testWorkloadName
		createStatefulSet(ctx, ns, name, nil)
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts)).To(Succeed())
		Expect(sts.Spec.Replicas).NotTo(BeNil())
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		resourceVersion := sts.ResourceVersion

		replicas := int32(1)
		guide := createGuide(ctx, ns, "default-replicas", &name, &replicas)

		result, err := reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, sts)).To(Succeed())
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		Expect(sts.ResourceVersion).To(Equal(resourceVersion))
		expectCondition(ctx, guide, metav1.ConditionTrue, ReasonReplicasSynced)
	})

	It("does not scale a StatefulSet of the same name in another namespace", func() {
		guideNS := createNamespace(ctx)
		otherNS := createNamespace(ctx)
		name := testWorkloadName
		createStatefulSet(ctx, otherNS, name, int32Ptr(1))
		replicas := int32(4)
		guide := createGuide(ctx, guideNS, "local-only", &name, &replicas)

		result, err := reconciler.Reconcile(ctx, requestFor(guide))
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(requeueOnNotFound))

		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: otherNS, Name: name}, sts)).To(Succeed())
		Expect(sts.Spec.Replicas).NotTo(BeNil())
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		expectCondition(ctx, guide, metav1.ConditionFalse, ReasonStatefulSetNotFound)
	})

	It("enqueues only Guides that name the StatefulSet", func() {
		ns := createNamespace(ctx)
		replicas := int32(1)
		name := testWorkloadName
		other := "other"
		matching := createGuide(ctx, ns, "matching", &name, &replicas)
		createGuide(ctx, ns, "unrelated", &other, &replicas)

		reqs := reconciler.mapStatefulSetToGuides(ctx, &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		})
		Expect(reqs).To(ConsistOf(requestFor(matching)))
	})
})

var namespaceCounter atomic.Uint64

func createNamespace(ctx context.Context) string {
	name := fmt.Sprintf("guide-%d", namespaceCounter.Add(1))
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	DeferCleanup(func() {
		err := k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		if err != nil && !errors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	})
	return name
}

func createGuide(ctx context.Context, namespace, name string, workload *string, replicas *int32) *testv1.Guide {
	guide := &testv1.Guide{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       testv1.GuideSpec{WorkloadName: workload, Replica: replicas},
	}
	Expect(k8sClient.Create(ctx, guide)).To(Succeed())
	return guide
}

func createStatefulSet(ctx context.Context, namespace, name string, replicas *int32) {
	labels := map[string]string{"app": name}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: appsv1.StatefulSetSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "app",
					Image: "example.com/app:test",
				}}},
			},
		},
	}
	Expect(k8sClient.Create(ctx, sts)).To(Succeed())
}

func requestFor(guide *testv1.Guide) reconcile.Request {
	return reconcile.Request{NamespacedName: client.ObjectKeyFromObject(guide)}
}

func expectCondition(ctx context.Context, guide *testv1.Guide, status metav1.ConditionStatus, reason string) {
	latest := &testv1.Guide{}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(guide), latest)).To(Succeed())
	condition := meta.FindStatusCondition(latest.Status.Conditions, ConditionAvailable)
	Expect(condition).NotTo(BeNil())
	Expect(condition.Status).To(Equal(status))
	Expect(condition.Reason).To(Equal(reason))
	Expect(latest.Status.Message).To(Equal(condition.Message))
	Expect(latest.Status.LastTransitionTime.IsZero()).To(BeFalse())
}

func int32Ptr(v int32) *int32 {
	return &v
}
