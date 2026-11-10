package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "kubeassemble.cn/guide/api/v1"
)

type guidePredicate struct{}

func (p *guidePredicate) Create(e event.CreateEvent) bool {
	guide, ok := e.Object.(*v1.Guide)
	if !ok {
		return false
	}
	klog.Infof("guidePredicate: %s/%s created, enqueue", guide.Namespace, guide.Name)
	return true
}

func (p *guidePredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

// Update enqueues only when the workload identity or replica count changes.
// Status-only writes, and updates where both objects omit the same fields,
// are ignored so a status update cannot loop. Nil spec fields are compared
// without dereferencing.
func (p *guidePredicate) Update(e event.UpdateEvent) bool {
	oldGuide, oldOK := e.ObjectOld.(*v1.Guide)
	newGuide, newOK := e.ObjectNew.(*v1.Guide)
	if !oldOK || !newOK {
		return false
	}
	if sameOptionalString(oldGuide.Spec.WorkloadName, newGuide.Spec.WorkloadName) &&
		sameOptionalInt32(oldGuide.Spec.Replica, newGuide.Spec.Replica) {
		klog.Infof("guidePredicate: guide %s/%s workload spec unchanged, ignore", newGuide.Namespace, newGuide.Name)
		return false
	}
	klog.Infof("guidePredicate: %s/%s updated, enqueue", newGuide.Namespace, newGuide.Name)
	return true
}

func (p *guidePredicate) Generic(_ event.GenericEvent) bool {
	return false
}

func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameOptionalInt32(a, b *int32) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// statefulSetReplicaPredicate reports StatefulSet creates, deletes, and
// replica changes. Other updates, such as status, do not need a Guide reconcile.
type statefulSetReplicaPredicate struct{}

func (p *statefulSetReplicaPredicate) Create(_ event.CreateEvent) bool {
	return true
}

func (p *statefulSetReplicaPredicate) Delete(_ event.DeleteEvent) bool {
	return true
}

func (p *statefulSetReplicaPredicate) Update(e event.UpdateEvent) bool {
	oldSTS, oldOK := e.ObjectOld.(*appsv1.StatefulSet)
	newSTS, newOK := e.ObjectNew.(*appsv1.StatefulSet)
	if !oldOK || !newOK {
		return true
	}
	return replicaCount(oldSTS.Spec.Replicas) != replicaCount(newSTS.Spec.Replicas)
}

func (p *statefulSetReplicaPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
