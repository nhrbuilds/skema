package skema

import "strings"

// ParseEnv validates an environ slice (os.Environ format, KEY=value entries)
// against the schema. Values are strings — numeric and bool fields need
// .Coerce(). Malformed entries (no "=") are skipped.
func ParseEnv[T any](v Validator[T], env []string) (T, error) {
	var zero T
	m := make(map[string]any, len(env))
	for _, kv := range env {
		key, val, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			continue
		}
		m[key] = val
	}
	out, err := v.ParseAny(m)
	if err != nil {
		return zero, err
	}
	return out, nil
}

// ParseEnvMap validates a KEY→VALUE map against the schema (same string
// semantics as ParseEnv).
func ParseEnvMap[T any](v Validator[T], env map[string]string) (T, error) {
	var zero T
	m := make(map[string]any, len(env))
	for k, val := range env {
		m[k] = val
	}
	out, err := v.ParseAny(m)
	if err != nil {
		return zero, err
	}
	return out, nil
}
