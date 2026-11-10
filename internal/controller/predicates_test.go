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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	testv1 "kubeassemble.cn/guide/api/v1"
)

var _ = Describe("guidePredicate", func() {
	p := &guidePredicate{}

	It("enqueues a create even when the spec is incomplete", func() {
		Expect(p.Create(event.CreateEvent{Object: &testv1.Guide{}})).To(BeTrue())
	})

	It("ignores objects that are not Guides", func() {
		Expect(p.Create(event.CreateEvent{Object: &corev1.Pod{}})).To(BeFalse())
		Expect(p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &testv1.Guide{}})).To(BeFalse())
	})

	It("ignores deletes", func() {
		Expect(p.Delete(event.DeleteEvent{Object: &testv1.Guide{}})).To(BeFalse())
	})

	It("does not panic when spec pointers are nil and ignores an unchanged incomplete spec", func() {
		oldGuide := &testv1.Guide{}
		newGuide := &testv1.Guide{}
		newGuide.Status.Message = "status only"
		Expect(func() {
			p.Update(event.UpdateEvent{ObjectOld: oldGuide, ObjectNew: newGuide})
		}).NotTo(Panic())
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldGuide, ObjectNew: newGuide})).To(BeFalse())
	})

	It("enqueues when a spec field is cleared", func() {
		replicas := int32(1)
		name := testWorkloadName
		oldGuide := &testv1.Guide{Spec: testv1.GuideSpec{Replica: &replicas, WorkloadName: &name}}
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldGuide, ObjectNew: &testv1.Guide{}})).To(BeTrue())
	})

	It("enqueues when the replica count changes", func() {
		before := int32(1)
		after := int32(2)
		name := testWorkloadName
		oldGuide := &testv1.Guide{Spec: testv1.GuideSpec{Replica: &before, WorkloadName: &name}}
		newGuide := &testv1.Guide{Spec: testv1.GuideSpec{Replica: &after, WorkloadName: &name}}
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldGuide, ObjectNew: newGuide})).To(BeTrue())
	})

	It("ignores status-only updates", func() {
		replicas := int32(2)
		name := testWorkloadName
		oldGuide := &testv1.Guide{Spec: testv1.GuideSpec{Replica: &replicas, WorkloadName: &name}}
		newGuide := oldGuide.DeepCopy()
		newGuide.Status.Message = "updated"
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldGuide, ObjectNew: newGuide})).To(BeFalse())
	})
})

var _ = Describe("statefulSetReplicaPredicate", func() {
	p := &statefulSetReplicaPredicate{}

	It("enqueues creates and deletes", func() {
		sts := &appsv1.StatefulSet{}
		Expect(p.Create(event.CreateEvent{Object: sts})).To(BeTrue())
		Expect(p.Delete(event.DeleteEvent{Object: sts})).To(BeTrue())
	})

	It("ignores updates that do not change the replica count", func() {
		replicas := int32(2)
		oldSTS := &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &replicas}}
		newSTS := oldSTS.DeepCopy()
		newSTS.Status.ReadyReplicas = 2
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldSTS, ObjectNew: newSTS})).To(BeFalse())
	})

	It("treats a nil replica count as 1", func() {
		one := int32(1)
		oldSTS := &appsv1.StatefulSet{}
		newSTS := &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &one}}
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldSTS, ObjectNew: newSTS})).To(BeFalse())
	})

	It("enqueues when the replica count changes", func() {
		before := int32(1)
		after := int32(3)
		oldSTS := &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &before}}
		newSTS := &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &after}}
		Expect(p.Update(event.UpdateEvent{ObjectOld: oldSTS, ObjectNew: newSTS})).To(BeTrue())
	})
})
