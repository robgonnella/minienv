// Package main generates schema/schema.json, the JSON schema editors use to
// complete and validate the x-minienv extension fields in a compose file.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/robgonnella/minienv/internal/config"
)

const (
	schemaDir  = "schema"
	schemaPath = "schema/schema.json"
	// Owner-writable only: this is a generated artifact checked into the repo.
	schemaDirPerm = 0o750

	composeSpecRef = "https://raw.githubusercontent.com/compose-spec/" +
		"compose-go/master/schema/compose-spec.json"
	composeServiceRef = composeSpecRef + "#/$defs/service"
)

func writeFile(filepath string, data []byte) error {
	// #nosec G304 -- filepath is a package constant, not user input.
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath, err)
	}

	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close file %s: %s", filepath, err)
		}
	}()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath, err)
	}

	return nil
}

// buildSchema wraps the official compose spec rather than restating it, so the
// extension fields are the only thing this repo has to keep in sync.
func buildSchema(r *jsonschema.Reflector) jsonschema.Schema {
	xMiniEnvSchema := r.Reflect(config.XMiniEnv{})
	xMiniEnvK8sServiceSchema := r.Reflect(config.XMiniEnvK8sService{})
	maps.Copy(xMiniEnvSchema.Definitions, xMiniEnvK8sServiceSchema.Definitions)

	serviceDefProperties := jsonschema.NewProperties()
	serviceDefProperties.Set(
		config.K8sServiceExtension,
		&jsonschema.Schema{Ref: xMiniEnvK8sServiceSchema.Ref},
	)

	serviceSchema := jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			{
				Description: "The official docker-compose service spec",
				Ref:         composeServiceRef,
			},
		},
		Properties: serviceDefProperties,
	}

	serviceDef := map[string]*jsonschema.Schema{"service": &serviceSchema}

	maps.Copy(xMiniEnvSchema.Definitions, serviceDef)

	servicesSchema := jsonschema.Schema{
		Description: "Wrapped docker-compose service spec with minienv extensions",
		PatternProperties: map[string]*jsonschema.Schema{
			"^[a-zA-Z0-9._-]+$": {
				Ref: "#/$defs/service",
			},
		},
	}

	schemaProperties := jsonschema.NewProperties()
	schemaProperties.Set(
		config.TopLevelExtension,
		&jsonschema.Schema{Ref: xMiniEnvSchema.Ref},
	)
	schemaProperties.Set("services", &servicesSchema)

	return jsonschema.Schema{
		ID:          "https://github.com/robgonnella/minienv/schema/schema.json",
		Version:     "https://json-schema.org/draft/2020-12/schema",
		Description: "Minienv schema wrapping official docker-compose schema but with minienv extensions", //nolint:lll
		AllOf: []*jsonschema.Schema{
			{
				Description: "The official docker-compose spec",
				Ref:         composeSpecRef,
			},
		},
		Definitions: xMiniEnvSchema.Definitions,
		Properties:  schemaProperties,
	}
}

func main() {
	r := new(jsonschema.Reflector)

	if err := r.AddGoComments(
		"github.com/robgonnella/minienv",
		"./",
	); err != nil {
		log.Fatalf("failed to add go comments to schema: %s", err)
	}

	schema := buildSchema(r)

	schemaData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("failed to generate schema: %s", err)
	}

	if err := os.MkdirAll(schemaDir, schemaDirPerm); err != nil {
		log.Fatalf("failed to create schema directory: %s", err)
	}

	if err := writeFile(schemaPath, schemaData); err != nil {
		log.Fatalf("failed to create %s: %s", schemaPath, err)
	}
}
