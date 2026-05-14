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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type podIPPredicate struct{}

func (p *podIPPredicate) Create(e event.CreateEvent) bool {
	pod := e.Object.(*corev1.Pod)
	if pod.Status.PodIP == "" {
		klog.V(4).Infof("podIPPredicate: pod %s/%s has no IP on create, skip", pod.Namespace, pod.Name)
		return false
	}
	if pod.Annotations != nil {
		if ip, ok := pod.Annotations[PodIPAnnotationKey]; ok && ip == pod.Status.PodIP {
			return false
		}
	}
	klog.Infof("podIPPredicate: pod %s/%s created with IP %s, enqueue", pod.Namespace, pod.Name, pod.Status.PodIP)
	return true
}

func (p *podIPPredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

func (p *podIPPredicate) Update(e event.UpdateEvent) bool {
	oldPod := e.ObjectOld.(*corev1.Pod)
	newPod := e.ObjectNew.(*corev1.Pod)

	// Only care when Pod gets an IP for the first time
	if oldPod.Status.PodIP != newPod.Status.PodIP && newPod.Status.PodIP != "" {
		if newPod.Annotations != nil {
			if ip, ok := newPod.Annotations[PodIPAnnotationKey]; ok && ip == newPod.Status.PodIP {
				return false
			}
		}
		klog.Infof("podIPPredicate: pod %s/%s got IP %s, enqueue", newPod.Namespace, newPod.Name, newPod.Status.PodIP)
		return true
	}
	return false
}

func (p *podIPPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
