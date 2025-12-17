package controller

import (
	"k8s.io/klog/v2"
	v1 "kubeassemble.cn/guide/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type guidePredicate struct{}

func (p *guidePredicate) Create(e event.CreateEvent) bool {
	guide := e.Object.(*v1.Guide)
	if guide.Spec.Replica == nil || guide.Spec.WorkloadName == nil {
		klog.V(4).Infof("guidePredicate: guide %s/%s replicas or workloadName is nil", guide.Namespace, guide.Name)
		return false
	}
	klog.V(4).Infof("guidePredicate: %s/%s new creation, enqueue", guide.Namespace, guide.Name)
	return true
}

func (p *guidePredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

// Update
func (p *guidePredicate) Update(e event.UpdateEvent) bool {
	oldGuide := e.ObjectOld.(*v1.Guide)
	newGuide := e.ObjectNew.(*v1.Guide)

	// first time to be labeled
	if oldGuide.Spec.Replica == newGuide.Spec.Replica && oldGuide.Spec.WorkloadName == newGuide.Spec.WorkloadName {
		klog.V(4).Infof("guidePredicate: guide %s/%s not update, ignore", newGuide.Namespace, newGuide.Name)
		return false
	}
	klog.V(4).Infof("guidePredicate: %s/%s updated, enqueue", newGuide.Namespace, newGuide.Name)
	return true
}

func (p *guidePredicate) Generic(_ event.GenericEvent) bool {
	return false
}
