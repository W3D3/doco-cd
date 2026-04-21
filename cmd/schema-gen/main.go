// Command schema-gen generates the JSON Schema for the doco-cd deployment
// configuration file (.doco-cd.yaml / .doco-cd.yml) from the DeployConfig Go
// struct and writes it to schemas/doco-cd.schema.json.
//
// Usage:
//
//	go run ./cmd/schema-gen
//
//go:generate go run .
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/invopop/jsonschema"

	"github.com/kimdre/doco-cd/internal/config"
)

const (
	schemaID    = "https://raw.githubusercontent.com/kimdre/doco-cd/main/schemas/doco-cd.schema.json"
	schemaTitle = "doco-cd deployment configuration"
	schemaDesc  = "Schema for the doco-cd deployment configuration file (.doco-cd.yaml / .doco-cd.yml)"
)

func main() {
	// Locate the repository root relative to this source file so that the
	// generator can be invoked from any working directory.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("could not determine source file path")
	}

	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	outPath := filepath.Join(repoRoot, "schemas", "doco-cd.schema.json")

	schema := buildSchema()

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal schema: %v", err)
	}

	// Ensure the output directory exists.
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		log.Fatalf("failed to write schema: %v", err)
	}

	fmt.Printf("schema written to %s\n", outPath)
}

// buildSchema generates the JSON Schema for DeployConfig and adds the
// conditional name-requirement logic that cannot be expressed as struct tags.
func buildSchema() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		// Only mark a field as required when it carries jsonschema:"required".
		// Every DeployConfig field is optional at the top level because the
		// name requirement is conditional on auto_discover not being true.
		RequiredFromJSONSchemaTags: true,
		// Inline nested anonymous structs instead of using $defs references.
		DoNotReference: true,
	}

	schema := r.Reflect(&config.DeployConfig{})

	// Override metadata set by the reflector.
	schema.Version = "http://json-schema.org/draft-07/schema#"
	schema.ID = jsonschema.ID(schemaID)
	schema.Title = schemaTitle
	schema.Description = schemaDesc

	// The reflector wraps the result in a top-level $ref when DoNotReference is
	// false, but with DoNotReference true it inlines everything into the root.
	// Unwrap the $defs wrapper if present so the schema root IS the object.
	if schema.Ref != "" {
		// Should not happen with DoNotReference: true, but guard anyway.
		log.Fatal("unexpected $ref at schema root; adjust generator options")
	}

	// Add the conditional: name is required unless auto_discover is explicitly true.
	addNameConditional(schema)

	return schema
}

// addNameConditional appends an if/then clause to schema that makes `name`
// required whenever `auto_discover` is absent or false.
func addNameConditional(schema *jsonschema.Schema) {
	// if: auto_discover is NOT explicitly set to true
	// then: name is required
	//
	// Expressed as:
	//   if:
	//     not:
	//       properties:
	//         auto_discover:
	//           const: true
	//       required: [auto_discover]
	//   then:
	//     required: [name]

	autoDiscoverConst := &jsonschema.Schema{}
	autoDiscoverConst.Const = true

	ifProps := jsonschema.NewProperties()
	ifProps.Set("auto_discover", autoDiscoverConst)

	ifSchema := &jsonschema.Schema{
		Not: &jsonschema.Schema{
			Properties: ifProps,
			Required:   []string{"auto_discover"},
		},
	}

	thenSchema := &jsonschema.Schema{
		Required: []string{"name"},
	}

	schema.If = ifSchema
	schema.Then = thenSchema
}
