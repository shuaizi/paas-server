package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	HandleMap = map[string]admission.Handler{}
)

func SetupWebhookWithManager(mgr manager.Manager, handleMap map[string]webhook.AdmissionHandler) error {

	for _, handler := range handleMap {
		if _, err := DeployDecoderInto(admission.NewDecoder(mgr.GetScheme()), handler); err != nil {
			return err
		}
	}

	for name, handler := range handleMap {
		mgr.GetWebhookServer().Register(name, &admission.Webhook{Handler: handler})
	}
	return nil
}
