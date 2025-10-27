package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

func generateSampleEnvFile(cfg any) string {
	var (
		val    reflect.Value = reflect.ValueOf(cfg)
		typ    reflect.Type  = reflect.TypeOf(cfg)
		output strings.Builder
	)

	for i := range val.NumField() {
		var (
			field  reflect.StructField = typ.Field(i)
			envTag string              = field.Tag.Get("env")
		)

		if field.Type.Kind() == reflect.Struct {
			output.WriteString(generateSampleEnvFile(reflect.New(field.Type).Elem().Interface()))
			continue
		}

		if envTag != "" {
			var (
				parts      []string = strings.Split(envTag, ",")
				envKey     string   = parts[0]
				defaultVal string
				required   bool
				value      string
			)

			for _, part := range parts[1:] {
				if strings.HasPrefix(part, "default=") {
					defaultVal = strings.TrimPrefix(part, "default=")
				}

				if part == "required=true" {
					required = true
				}
			}

			value = defaultVal
			if required && defaultVal == "" {
				value = ""
			}

			output.WriteString(envKey + "=" + value + "\n")
		}
	}

	return output.String()
}

// GenerateSampleEnvFile generates a sample .env file based on the provided configuration struct.
// It writes the output to the specified path.
func GenerateSampleEnvFile(path string) error {
	if err := os.WriteFile(path, []byte(generateSampleEnvFile(Configuration{})), 0644); err != nil {
		return fmt.Errorf("failed to write sample .env file: %w", err)
	}

	return nil
}
