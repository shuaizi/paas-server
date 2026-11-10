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

const (
	injectedAnnotation = "example.com/injected"
	initFileDir        = "/etc/container"
	initFileName       = "init"
)

type PodMutator struct {
	Decoder admission.Decoder
	// now 是准入时取“当前时间”的时钟，测试可替换。为空时使用 time.Now。
	now func() time.Time
}

// Handle 在 Pod 创建时打上标记，并为每个还没有 postStart 的业务容器
// 写入 postStart：把本次准入的 UTC 时间落到 /etc/container/init。
func (m *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	start := time.Now()
	defer func() {
		klog.Infof("PodMutator.Handle: req: %v, cost %v", req.Name, time.Since(start))
	}()

	if req.Operation != admissionv1.Create {
		return admission.Allowed("")
	}
	pod := &corev1.Pod{}
	if err := m.Decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	deepCopy := pod.DeepCopy()
	if deepCopy.Annotations == nil {
		deepCopy.Annotations = map[string]string{}
	}
	if _, ok := deepCopy.Annotations[injectedAnnotation]; !ok {
		deepCopy.Annotations[injectedAnnotation] = "true"
	}
	m.ensurePostStart(deepCopy)

	byteToPatch, err := json.Marshal(deepCopy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, byteToPatch)
}

// ensurePostStart 只改 spec.containers。initContainers 不在这次创建事件的处理范围内。
// 容器已经配置了 postStart 时保留原配置，避免覆盖用户自己的启动钩子。
func (m *PodMutator) ensurePostStart(pod *corev1.Pod) {
	command := initFileCommand(m.currentTime().UTC())
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Lifecycle != nil && container.Lifecycle.PostStart != nil {
			klog.Infof("Pod %s/%s container %s already has postStart, skip", pod.Namespace, pod.Name, container.Name)
			continue
		}
		if container.Lifecycle == nil {
			container.Lifecycle = &corev1.Lifecycle{}
		}
		container.Lifecycle.PostStart = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: append([]string(nil), command...)},
		}
	}
}

func (m *PodMutator) currentTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// initFileCommand 生成 postStart 命令。时间取自 webhook 处理这次创建的时刻，
// 这样文件内容不依赖镜像里是否有 date。
func initFileCommand(now time.Time) []string {
	script := fmt.Sprintf("mkdir -p %s && printf '%%s\\n' '%s' > %s/%s",
		initFileDir, now.Format(time.RFC3339), initFileDir, initFileName)
	return []string{"/bin/sh", "-c", script}
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
