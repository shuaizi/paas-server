package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type PodMutator struct {
	Decoder admission.Decoder
}

// Handle 实现 MutatingWebhook
func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {

	start := time.Now()
	defer func() {
		klog.Infof("PodMutator.Handle: req: %v, cost %v", req.Name, time.Now().After(start))
	}()

	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	pod := &corev1.Pod{}
	if err := m.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	deepCopy := pod.DeepCopy()

	// 确保 annotations 不为 nil
	if deepCopy.Annotations == nil {
		deepCopy.Annotations = map[string]string{}
	}

	// 如果已存在则不处理（幂等）
	if _, ok := deepCopy.Annotations["example.com/injected"]; !ok {
		deepCopy.Annotations["example.com/injected"] = "true"
	}

	byteToPatch, err := json.Marshal(deepCopy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, byteToPatch)
}

// InjectDecoder 注入 decoder
func (m *PodMutator) InjectDecoder(d admission.Decoder) error {
	if d == nil {
		return fmt.Errorf("nil decoder")
	}
	m.Decoder = d
	return nil
}

func init() {
	HandleMap["/mutate-v1-pod"] = &PodMutator{}
}
