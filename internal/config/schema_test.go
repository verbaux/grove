package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSchemaCoversConfig guards against drift: every field on Config must
// appear as a property in groverc.schema.json. Add a field, add it to the
// schema — this test fails until you do.
func TestSchemaCoversConfig(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "groverc.schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		if _, ok := schema.Properties[name]; !ok {
			t.Errorf("Config field %q (json %q) is missing from groverc.schema.json", typ.Field(i).Name, name)
		}
	}
}
