package detect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GoProvider detects Go projects.
type GoProvider struct{}

func (p *GoProvider) Name() string { return "go" }

func (p *GoProvider) Detect(root string) bool {
	return fileExists(root, "go.mod")
}

func (p *GoProvider) Plan(root string) (*BuildPlan, error) {
	modName := parseModuleName(root)
	binaryName := moduleName(modName)
	if binaryName == "" {
		binaryName = "server"
	}

	// Detect main package location
	buildTarget := detectGoMainTarget(root)

	return &BuildPlan{
		Provider:    "go",
		Runtime:     "golang:1.25-alpine",
		BinaryName:  binaryName,
		Port:        8080,
		BuildTarget: buildTarget,
	}, nil
}

// parseModuleName reads go.mod and extracts the module path.
func parseModuleName(root string) string {
	f, err := os.Open(root + "/go.mod")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// moduleName returns the last segment of a module path.
func moduleName(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// detectGoMainTarget finds the directory containing `package main`.
// Priority: cmd/<matching-name> → cmd/<first> → root → ./...
func detectGoMainTarget(root string) string {
	modName := parseModuleName(root)
	lastSeg := moduleName(modName)

	mainDirs := findMainPackages(root)

	if len(mainDirs) == 0 {
		return "./..."
	}

	// Check cmd/ subdirectories first
	var cmdDirs []string
	for _, d := range mainDirs {
		rel, _ := filepath.Rel(root, d)
		if strings.HasPrefix(rel, "cmd") {
			cmdDirs = append(cmdDirs, rel)
		}
	}

	if len(cmdDirs) > 0 {
		sort.Strings(cmdDirs)

		// Try to match module name
		if lastSeg != "" {
			for _, d := range cmdDirs {
				if filepath.Base(d) == lastSeg {
					return "./" + d
				}
			}
		}

		// Return first alphabetically
		return "./" + cmdDirs[0]
	}

	// Root has package main
	for _, d := range mainDirs {
		if d == root {
			return "."
		}
	}

	// Use first found
	rel, _ := filepath.Rel(root, mainDirs[0])
	return "./" + rel
}

// findMainPackages walks root (max depth 3) looking for .go files with `package main`.
func findMainPackages(root string) []string {
	var dirs []string
	seen := make(map[string]bool)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Limit depth to 3
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(os.PathSeparator))
		if depth > 3 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip vendor and hidden dirs
		if info.IsDir() && (info.Name() == "vendor" || (len(info.Name()) > 0 && info.Name()[0] == '.')) {
			return filepath.SkipDir
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		if hasPackageMain(path) {
			dir := filepath.Dir(path)
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}

		return nil
	})

	return dirs
}

// hasPackageMain checks if a .go file declares `package main` in its first 50 lines.
func hasPackageMain(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineCount := 0
	for scanner.Scan() && lineCount < 50 {
		line := strings.TrimSpace(scanner.Text())
		if line == "package main" {
			return true
		}
		// Stop early if we hit a non-comment, non-blank line that isn't package main
		if line != "" && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") && !strings.HasPrefix(line, "*") && strings.HasPrefix(line, "package ") {
			return false
		}
		lineCount++
	}
	return false
}

// unused but available for future use
var _ = fmt.Sprintf
