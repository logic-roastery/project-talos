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

// fileExists checks if a file exists in the given directory.
func fileExists(root, name string) bool {
	_, err := os.Stat(root + "/" + name)
	return err == nil
}
