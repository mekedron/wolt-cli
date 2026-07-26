package mcpserver

import "strings"

func containsWarning(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}
