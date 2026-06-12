package detect

import (
	"fmt"
	"os"
)

// providers is the detection order — first match wins.
var providers = []Provider{
	&StaticProvider{},
	&NodeProvider{},
	&GoProvider{},
	&JavaProvider{},
}

// Detect scans root for sentinel files and returns a BuildPlan from the first matching provider.
func Detect(root string) (*BuildPlan, error) {
	for _, p := range providers {
		if p.Detect(root) {
			plan, err := p.Plan(root)
			if err != nil {
				return nil, fmt.Errorf("provider %s: %w", p.Name(), err)
			}
			return plan, nil
		}
	}
	return nil, fmt.Errorf("unsupported project type: no package.json, go.mod, pom.xml, build.gradle, or index.html found")
}

// providerMap enables name-based lookup for forced project type selection.
var providerMap = map[string]Provider{
	"static": &StaticProvider{},
	"node":   &NodeProvider{},
	"go":     &GoProvider{},
	"java":   &JavaProvider{},
}

// DetectAs runs detection with an optional provider override.
// If forceProvider is non-empty, that provider's Plan() is called directly (skip detection).
// If forceProvider is "", the existing Detect() auto-detection runs.
func DetectAs(root string, forceProvider string) (*BuildPlan, error) {
	if forceProvider == "" {
		return Detect(root)
	}
	p, ok := providerMap[forceProvider]
	if !ok {
		return nil, fmt.Errorf("unknown project type %q", forceProvider)
	}
	return p.Plan(root)
}

// fileExists checks if a file exists in the given directory.
func fileExists(root, name string) bool {
	_, err := os.Stat(root + "/" + name)
	return err == nil
}
