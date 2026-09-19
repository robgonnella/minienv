package compose

import "strings"

func Templated(value string) bool {
	return strings.Contains(strings.ReplaceAll(value, "$$", ""), "$")
}

func LiteralEnvKeys(rawEnvironment any) map[string]bool {
	switch env := rawEnvironment.(type) {
	case []any:
		return literalListEnvKeys(env)
	case map[string]any:
		return literalMapEnvKeys(env)
	default:
		return map[string]bool{}
	}
}

func literalListEnvKeys(env []any) map[string]bool {
	literal := map[string]bool{}

	for _, entry := range env {
		s, ok := entry.(string)
		if !ok {
			continue
		}

		key, value, found := strings.Cut(s, "=")
		if found && !Templated(value) {
			literal[key] = true
		}
	}

	return literal
}

func literalMapEnvKeys(env map[string]any) map[string]bool {
	literal := map[string]bool{}

	for key, value := range env {
		switch v := value.(type) {
		case nil:
		case string:
			if !Templated(v) {
				literal[key] = true
			}
		default:
			literal[key] = true
		}
	}

	return literal
}
