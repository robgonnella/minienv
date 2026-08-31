package main

import (
	"encoding/json"
	"log"
	"maps"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/robgonnella/minienv/internal/config"
)

func writeFile(filepath string, data []byte) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func main() {
	r := new(jsonschema.Reflector)

	if err := r.AddGoComments("github.com/robgonnella/minienv", "./"); err != nil {
		log.Fatalf("failed to add go comments to schema: %s", err)
	}

	xMiniEnvSchema := r.Reflect(config.XMiniEnv{})
	xMiniEnvK8sServiceSchema := r.Reflect(config.XMiniEnvK8sService{})
	maps.Copy(xMiniEnvSchema.Definitions, xMiniEnvK8sServiceSchema.Definitions)

	serviceDefProperties := jsonschema.NewProperties()
	serviceDefProperties.Set(
		"x-minienv-k8s-service",
		&jsonschema.Schema{Ref: xMiniEnvK8sServiceSchema.Ref},
	)

	serviceSchema := jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			{
				Description: "The official docker-compose service spec",
				Ref:         "https://raw.githubusercontent.com/compose-spec/compose-go/master/schema/compose-spec.json#/$defs/service",
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
	schemaProperties.Set("x-minienv", &jsonschema.Schema{Ref: xMiniEnvSchema.Ref})
	schemaProperties.Set("services", &servicesSchema)

	schema := jsonschema.Schema{
		ID:          "https://github.com/robgonnella/minienv/schema/schema.json",
		Version:     "https://json-schema.org/draft/2020-12/schema",
		Description: "Minienv schema wrapping official docker-compose schema but with minienv extensions",
		AllOf: []*jsonschema.Schema{
			{
				Description: "The official docker-compose spec",
				Ref:         "https://raw.githubusercontent.com/compose-spec/compose-go/master/schema/compose-spec.json",
			},
		},
		Definitions: xMiniEnvSchema.Definitions,
		Properties:  schemaProperties,
	}

	schemaData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("failed to generate schema: %s", err)
	}

	if err := os.MkdirAll("schema", 0751); err != nil {
		log.Fatalf("failed to create schema directory")
	}

	if err := writeFile(
		"schema/schema.json",
		schemaData,
	); err != nil {
		log.Fatalf("failed to create schema/schema.json: %s", err)
	}
}
