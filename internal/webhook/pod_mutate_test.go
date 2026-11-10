package webhook

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	gomodpatch "gomodules.xyz/jsonpatch/v2"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestInitFileCommandUsesAdmissionTime(t *testing.T) {
	now := time.Date(2026, 11, 10, 8, 32, 0, 0, time.FixedZone("CST", 8*3600))
	command := initFileCommand(now.UTC())
	if len(command) != 3 || command[0] != "/bin/sh" || command[1] != "-c" {
		t.Fatalf("command = %#v", command)
	}
	want := "mkdir -p /etc/container && printf '%s\\n' '2026-11-10T00:32:00Z' > /etc/container/init"
	if command[2] != want {
		t.Fatalf("script = %q, want %q", command[2], want)
	}
}

func TestPodMutatorAddsPostStartOnCreate(t *testing.T) {
	fixed := time.Date(2026, 11, 10, 8, 32, 0, 0, time.UTC)
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "team"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{
				Name:  "setup",
				Image: "example.com/setup:test",
			}},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "example.com/app:test",
					Lifecycle: &corev1.Lifecycle{
						PreStop: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"/bin/stop"}}},
					},
				},
				{
					Name:  "already",
					Image: "example.com/app:test",
					Lifecycle: &corev1.Lifecycle{
						PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{Command: []string{"/bin/existing"}}},
					},
				},
			},
		},
	}

	got := admitPod(t, pod, admissionv1.Create, fixed)

	if got.Annotations[injectedAnnotation] != "true" {
		t.Fatalf("annotation = %#v", got.Annotations)
	}
	app := got.Spec.Containers[0]
	if app.Lifecycle == nil || app.Lifecycle.PreStop == nil || app.Lifecycle.PreStop.Exec == nil {
		t.Fatal("preStop was dropped")
	}
	if gotCommand := app.Lifecycle.PreStop.Exec.Command; len(gotCommand) != 1 || gotCommand[0] != "/bin/stop" {
		t.Fatalf("preStop = %#v", gotCommand)
	}
	if app.Lifecycle.PostStart == nil || app.Lifecycle.PostStart.Exec == nil {
		t.Fatal("app container did not get postStart")
	}
	want := initFileCommand(fixed)
	if gotCommand := app.Lifecycle.PostStart.Exec.Command; !sameStrings(gotCommand, want) {
		t.Fatalf("postStart = %#v, want %#v", gotCommand, want)
	}

	already := got.Spec.Containers[1]
	if already.Lifecycle == nil || already.Lifecycle.PostStart == nil || already.Lifecycle.PostStart.Exec == nil {
		t.Fatal("existing postStart was removed")
	}
	if gotCommand := already.Lifecycle.PostStart.Exec.Command; len(gotCommand) != 1 || gotCommand[0] != "/bin/existing" {
		t.Fatalf("existing postStart = %#v", gotCommand)
	}
	if len(got.Spec.InitContainers) != 1 || got.Spec.InitContainers[0].Lifecycle != nil {
		t.Fatalf("init container = %#v", got.Spec.InitContainers)
	}
}

func TestPodMutatorIgnoresNonCreate(t *testing.T) {
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "team"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example.com/app:test"}}},
	}
	raw := mustMarshal(t, pod)
	resp := newMutator(time.Now()).Handle(context.Background(), admissionRequest(admissionv1.Update, raw))
	if !resp.Allowed {
		t.Fatalf("update was denied: %#v", resp.Result)
	}
	if len(resp.Patches) != 0 {
		t.Fatalf("update produced patches: %#v", resp.Patches)
	}
}

func TestPodMutatorRejectsUndecodablePod(t *testing.T) {
	resp := newMutator(time.Now()).Handle(context.Background(), admissionRequest(admissionv1.Create, []byte(`{"apiVersion":`)))
	if resp.Allowed {
		t.Fatal("malformed pod was allowed")
	}
}

func admitPod(t *testing.T, pod *corev1.Pod, op admissionv1.Operation, now time.Time) *corev1.Pod {
	t.Helper()
	raw := mustMarshal(t, pod)
	resp := newMutator(now).Handle(context.Background(), admissionRequest(op, raw))
	if !resp.Allowed {
		t.Fatalf("admission denied: %#v", resp.Result)
	}
	patched, err := applyJSONPatch(raw, resp.Patches)
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	got := &corev1.Pod{}
	if err := json.Unmarshal(patched, got); err != nil {
		t.Fatalf("unmarshal patched pod: %v", err)
	}
	return got
}

func newMutator(now time.Time) *PodMutator {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return &PodMutator{
		Decoder: admission.NewDecoder(scheme),
		now:     func() time.Time { return now },
	}
}

func admissionRequest(op admissionv1.Operation, raw []byte) admission.Request {
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: op,
		Object:    runtime.RawExtension{Raw: raw},
		Name:      "demo",
		Namespace: "team",
	}}
}

func mustMarshal(t *testing.T, pod *corev1.Pod) []byte {
	t.Helper()
	raw, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal pod: %v", err)
	}
	return raw
}

func applyJSONPatch(original []byte, operations []gomodpatch.JsonPatchOperation) ([]byte, error) {
	encoded, err := json.Marshal(operations)
	if err != nil {
		return nil, err
	}
	patch, err := jsonpatch.DecodePatch(encoded)
	if err != nil {
		return nil, err
	}
	return patch.Apply(original)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
