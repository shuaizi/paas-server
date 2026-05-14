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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Pod Controller", func() {
	const (
		podName      = "test-pod"
		podNamespace = "default"
		podIP        = "10.0.0.1"
	)

	ctx := context.Background()

	namespacedName := types.NamespacedName{
		Name:      podName,
		Namespace: podNamespace,
	}

	var reconciler *PodReconciler

	BeforeEach(func() {
		reconciler = &PodReconciler{
			Client: k8sClient,
		}
	})

	AfterEach(func() {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx, namespacedName, pod)
		if err == nil {
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
		}
	})

	newTestPod := func(ip string, annotations map[string]string) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        podName,
				Namespace:   podNamespace,
				Annotations: annotations,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "busybox",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		if ip != "" {
			pod.Status.PodIP = ip
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		}

		return pod
	}

	Context("When Pod has no IP", func() {
		It("should skip without error", func() {
			newTestPod("", nil)

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, namespacedName, pod)).To(Succeed())
			Expect(pod.Annotations).NotTo(HaveKey(PodIPAnnotationKey))
		})
	})

	Context("When Pod has an IP", func() {
		It("should add the IP annotation", func() {
			newTestPod(podIP, nil)

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, namespacedName, pod)).To(Succeed())
			Expect(pod.Annotations).To(HaveKeyWithValue(PodIPAnnotationKey, podIP))
		})

		It("should preserve existing annotations", func() {
			newTestPod(podIP, map[string]string{"existing-key": "existing-value"})

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, namespacedName, pod)).To(Succeed())
			Expect(pod.Annotations).To(HaveKeyWithValue(PodIPAnnotationKey, podIP))
			Expect(pod.Annotations).To(HaveKeyWithValue("existing-key", "existing-value"))
		})
	})

	Context("When Pod already has correct IP annotation", func() {
		It("should be a no-op", func() {
			newTestPod(podIP, map[string]string{PodIPAnnotationKey: podIP})

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("When Pod has stale IP annotation", func() {
		It("should update the annotation to the new IP", func() {
			newTestPod(podIP, map[string]string{PodIPAnnotationKey: "10.0.0.99"})

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, namespacedName, pod)).To(Succeed())
			Expect(pod.Annotations).To(HaveKeyWithValue(PodIPAnnotationKey, podIP))
		})
	})

	Context("When Pod does not exist", func() {
		It("should return without error", func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: podNamespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
