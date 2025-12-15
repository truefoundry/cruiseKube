package config

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"
)

type TaskConfig struct {
	Enabled  bool
	Schedule string
	Metadata map[string]interface{}
}

func (tc *TaskConfig) ConvertMetadataToStruct(target interface{}) error {
	if tc.Metadata == nil {
		return nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		WeaklyTypedInput: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(tc.Metadata); err != nil {
		return fmt.Errorf("failed to decode metadata: %w", err)
	}

	return nil
}
