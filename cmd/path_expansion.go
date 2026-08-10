package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/verbaux/grove/internal/config"
)

func expansionWarningValues(warnings []config.PathExpansionWarning) []string {
	values := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if warning.Path != "" {
			values = append(values, warning.Path)
		} else {
			values = append(values, warning.Pattern)
		}
	}
	return values
}

func actionableExpansionWarnings(warnings []config.PathExpansionWarning) []config.PathExpansionWarning {
	result := make([]config.PathExpansionWarning, 0, len(warnings))
	for _, warning := range warnings {
		if warning.Reason == config.PathPatternUnmatched && !config.HasGlobMeta(warning.Pattern) {
			continue
		}
		result = append(result, warning)
	}
	return result
}

func printExpansionWarnings(field string, warnings []config.PathExpansionWarning) {
	for _, warning := range warnings {
		value := warning.Pattern
		if warning.Path != "" {
			value = warning.Path
		}
		fmt.Fprintf(os.Stderr, "  warning: skipping %s %s: %s\n", field, value, warning.Reason)
	}
}

// inspectionPaths keeps missing literal entries visible to health checks while
// avoiding phantom paths for unmatched glob patterns.
func inspectionPaths(expansion config.PathExpansion) []string {
	paths := append([]string(nil), expansion.Paths...)
	for _, warning := range expansion.Warnings {
		if warning.Reason == config.PathPatternUnmatched && !config.HasGlobMeta(warning.Pattern) {
			paths = append(paths, warning.Pattern)
		}
	}
	sort.Strings(paths)
	return paths
}
