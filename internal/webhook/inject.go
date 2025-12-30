package webhook

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type InitDecoder interface {
	InjectDecoder(d admission.Decoder) error
}

func DeployDecoderInto(decoder admission.Decoder, i interface{}) (bool, error) {
	if s, ok := i.(InitDecoder); ok {
		return true, s.InjectDecoder(decoder)
	}
	return false, nil
}
