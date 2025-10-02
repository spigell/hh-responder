package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Source describes how to load a secret value.
type Source struct {
	// Name is used in error messages to give more context about the secret.
	Name string
	// Value is an inline secret value provided via configuration or flags.
	Value string
	// File points to a file containing the secret value. When set it takes
	// precedence over Value.
	File string
}

// Load returns the resolved secret value from the provided source. When File is
// set it takes precedence over Value. The returned secret is always trimmed. An
// error is returned when neither File nor Value contain a usable secret.
func Load(src Source) (string, error) {
	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = "secret"
	}

	file := strings.TrimSpace(src.File)
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s from file %q: %w", name, file, err)
		}
		src.Value = string(data)
		src.File = file
	}

	secret := strings.TrimSpace(src.Value)
	if secret == "" {
		if src.File != "" {
			return "", fmt.Errorf("%s file %q is empty", name, src.File)
		}
		return "", fmt.Errorf("%s is not configured", name)
	}

	return secret, nil
}
