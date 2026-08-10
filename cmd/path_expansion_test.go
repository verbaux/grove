package cmd

import (
	"reflect"
	"testing"

	"github.com/verbaux/grove/internal/config"
)

func TestActionableExpansionWarningsIgnoreMissingLiterals(t *testing.T) {
	warnings := []config.PathExpansionWarning{
		{Pattern: "node_modules", Reason: config.PathPatternUnmatched},
		{Pattern: "apps/*/node_modules", Reason: config.PathPatternUnmatched},
		{Pattern: "packages/*/dist", Path: "packages/bad/dist", Reason: config.PathMatchNotDirectory},
	}

	got := actionableExpansionWarnings(warnings)
	want := warnings[1:]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actionableExpansionWarnings() = %#v, want %#v", got, want)
	}
}
