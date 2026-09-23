package resolver

import (
	"reflect"
	"strconv"

	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/errs"
)

func decodeServiceExtension(ext any, name string, out any) error {
	decoderConfig := &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.DecodeHookFuncType(stringToBool),
		Result:     out,
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to create decoder: %w",
			err,
		)
	}

	if err := decoder.Decode(ext); err != nil {
		return errs.Errorf(
			ErrExtensionDecode,
			"failed to decode extension %s: %w",
			name,
			err,
		)
	}

	return nil
}

func stringToBool(from, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to.Kind() != reflect.Bool {
		return data, nil
	}

	raw, ok := data.(string)
	if !ok {
		return data, nil
	}

	if raw == "" {
		return false, nil
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errs.Errorf(
			ErrExtensionDecode,
			"expected a bool, got %q: %w",
			raw,
			err,
		)
	}

	return parsed, nil
}
