package loader

import (
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
)

func (w *serviceDirWalk) hostEnv(
	project *config.ComposeProject,
) deployer.HostEnv {
	hostEnv := deployer.HostEnv{}

	for name, svc := range project.Services {
		keys := hostEnvKeys(svc.Environment, w.rawEnv[name])
		if len(keys) > 0 {
			hostEnv[name] = keys
		}
	}

	return hostEnv
}

func hostEnvKeys(resolved types.MappingWithEquals, raw any) []string {
	literal := literalEnvKeys(raw)
	keys := []string{}

	for key, value := range resolved {
		if value == nil || literal[key] {
			continue
		}

		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func literalEnvKeys(raw any) map[string]bool {
	switch env := raw.(type) {
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
		if found && !templated(value) {
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
			if !templated(v) {
				literal[key] = true
			}
		default:
			literal[key] = true
		}
	}

	return literal
}

func templated(value string) bool {
	return strings.Contains(strings.ReplaceAll(value, "$$", ""), "$")
}
