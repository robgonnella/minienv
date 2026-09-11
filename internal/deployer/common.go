package deployer

import "github.com/robgonnella/minienv/internal/config"

func PublishedNames(published []config.NgrokEndpointConfig) []string {
	names := make([]string, 0, len(published))
	for _, svc := range published {
		names = append(names, svc.EndpointName)
	}

	return names
}
