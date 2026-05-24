package config

import (
	"strings"
	"testing"
)

func TestSchemaURL(t *testing.T) {
	tests := map[string]string{
		"v0.6.0":                    "/v0.6.0/",
		"v1.2.3":                    "/v1.2.3/",
		"":                          "/main/",
		"v0.5.1-0.20260524-82dff55": "/main/", // dev pseudo-version
		"82dff55":                   "/main/", // commit hash
		"v0.6":                      "/main/", // not a full semver tag
	}
	for version, wantRef := range tests {
		got := SchemaURL(version)
		if !strings.Contains(got, wantRef) {
			t.Errorf("SchemaURL(%q) = %q, want it to contain %q", version, got, wantRef)
		}
		if !strings.HasSuffix(got, "/groverc.schema.json") {
			t.Errorf("SchemaURL(%q) = %q, missing schema filename", version, got)
		}
	}
}
