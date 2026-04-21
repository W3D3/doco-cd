package main

import (
	"encoding/json"
	"testing"

	"github.com/invopop/jsonschema"
)

func TestBuildSchema(t *testing.T) {
	schema := buildSchema()

	// Schema must be non-nil and have the correct type.
	if schema == nil {
		t.Fatal("buildSchema returned nil")
	}

	if schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", schema.Type)
	}

	// Metadata fields must be populated.
	if string(schema.ID) != schemaID {
		t.Errorf("expected $id %q, got %q", schemaID, schema.ID)
	}

	if schema.Title != schemaTitle {
		t.Errorf("expected title %q, got %q", schemaTitle, schema.Title)
	}

	// The conditional if/then must be present.
	if schema.If == nil || schema.Then == nil {
		t.Error("expected if/then conditional to be set")
	}

	// 'name' must be in then.required.
	if len(schema.Then.Required) == 0 || schema.Then.Required[0] != "name" {
		t.Errorf("expected then.required to contain 'name', got %v", schema.Then.Required)
	}

	// All expected top-level properties must be present.
	expected := []string{
		"name", "repository_url", "webhook_filter", "reference", "working_dir",
		"compose_files", "environment", "env_files", "remove_orphans", "prune_images",
		"force_recreate", "force_image_pull", "timeout", "build_opts", "destroy",
		"destroy_opts", "profiles", "external_secrets", "auto_discover", "auto_discover_opts",
	}

	for _, prop := range expected {
		if _, ok := schema.Properties.Get(prop); !ok {
			t.Errorf("expected property %q to be present in schema", prop)
		}
	}

	// 'Internal' must NOT appear in the schema (json:"-").
	if _, ok := schema.Properties.Get("Internal"); ok {
		t.Error("internal field 'Internal' must not be present in the schema")
	}

	// external_secrets must use oneOf for its additionalProperties.
	extSecSchema, ok := schema.Properties.Get("external_secrets")
	if !ok {
		t.Fatal("external_secrets property not found")
	}

	if extSecSchema.AdditionalProperties == nil {
		t.Fatal("external_secrets.additionalProperties must not be nil")
	}

	if len(extSecSchema.AdditionalProperties.OneOf) != 2 {
		t.Errorf("expected 2 oneOf variants for external_secrets value, got %d",
			len(extSecSchema.AdditionalProperties.OneOf))
	}

	// Schema must be serialisable to valid JSON.
	if _, err := json.Marshal(schema); err != nil {
		t.Errorf("schema must serialise to JSON without error: %v", err)
	}
}

func TestBuildSchema_AdditionalPropertiesFalse(t *testing.T) {
	schema := buildSchema()

	// The root schema must not allow extra keys.
	if schema.AdditionalProperties == nil {
		t.Fatal("root schema must have additionalProperties set")
	}

	ap := schema.AdditionalProperties
	// invopop/jsonschema represents additionalProperties:false as a boolean Schema.
	b, _ := json.Marshal(ap)
	if string(b) != "false" {
		t.Errorf("expected additionalProperties to be false, got %s", b)
	}
}

func TestAddNameConditional(t *testing.T) {
	base := &jsonschema.Schema{Type: "object"}
	addNameConditional(base)

	if base.If == nil {
		t.Fatal("if clause must be set after addNameConditional")
	}

	if base.Then == nil {
		t.Fatal("then clause must be set after addNameConditional")
	}

	if base.If.Not == nil {
		t.Fatal("if.not must be set")
	}

	if _, ok := base.If.Not.Properties.Get("auto_discover"); !ok {
		t.Error("if.not.properties must contain 'auto_discover'")
	}
}
