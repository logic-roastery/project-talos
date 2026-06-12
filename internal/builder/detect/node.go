package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// NodeProvider detects Node.js and Bun projects.
type NodeProvider struct{}

type packageJSON struct {
	Main            string            `json:"main"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
}

func (p *NodeProvider) Name() string { return "node" }

func (p *NodeProvider) Detect(root string) bool {
	return fileExists(root, "package.json")
}

func (p *NodeProvider) Plan(root string) (*BuildPlan, error) {
	pkg, err := parsePackageJSON(root)
	if err != nil {
		return nil, fmt.Errorf("parse package.json: %w", err)
	}

	plan := &BuildPlan{
		Provider: "node",
		Port:     3000,
	}

	// Detect runtime: Bun vs Node
	isBun := fileExists(root, "bun.lockb") || fileExists(root, "bunfig.toml")
	if isBun {
		plan.Runtime = "oven/bun:1-alpine"
	} else {
		plan.Runtime = "node:22-alpine"
	}

	// Detect package manager and install command
	pm := nodePackageManager(root, isBun)
	plan.Install = []string{nodeInstallCmd(root, isBun)}

	// Detect build step
	if _, ok := pkg.Scripts["build"]; ok {
		plan.Build = []string{pm + " run build"}
	}

	// Detect start command and port
	plan.StartCommand, plan.Port = detectStartCommand(pm, pkg)

	return plan, nil
}

func parsePackageJSON(root string) (*packageJSON, error) {
	data, err := os.ReadFile(root + "/package.json")
	if err != nil {
		return nil, err
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// nodePackageManager returns the package manager binary name.
func nodePackageManager(root string, isBun bool) string {
	if isBun {
		return "bun"
	}
	if fileExists(root, "pnpm-lock.yaml") {
		return "pnpm"
	}
	if fileExists(root, "yarn.lock") {
		return "yarn"
	}
	return "npm"
}

// nodeInstallCmd returns the install command for the detected package manager.
func nodeInstallCmd(root string, isBun bool) string {
	if isBun {
		return "bun install --frozen-lockfile"
	}
	if fileExists(root, "pnpm-lock.yaml") {
		return "corepack enable && pnpm install --frozen-lockfile"
	}
	if fileExists(root, "yarn.lock") {
		return "corepack enable && yarn install --frozen-lockfile"
	}
	if fileExists(root, "package-lock.json") {
		return "npm ci"
	}
	return "npm install"
}

// detectStartCommand determines the start command and port.
func detectStartCommand(pm string, pkg *packageJSON) (string, int) {
	// Priority 1: scripts.start
	if startCmd, ok := pkg.Scripts["start"]; ok {
		port := parsePortFromCmd(startCmd)
		if port > 0 {
			return startCmd, port
		}
		return startCmd, 3000
	}

	// Priority 2: Framework detection from dependencies
	allDeps := mergeDeps(pkg)

	if _, ok := allDeps["next"]; ok {
		return pm + " next start", 3000
	}
	if _, ok := allDeps["nuxt"]; ok {
		return "node .output/server/index.mjs", 3000
	}
	if _, ok := allDeps["@angular/core"]; ok {
		return pm + " ng serve --host 0.0.0.0", 4200
	}
	if _, ok := allDeps["vite"]; ok {
		if !hasServerDep(allDeps) {
			return "", 80 // static output, use nginx
		}
	}
	if hasServerDep(allDeps) {
		return nodeStartMain(pkg), 3000
	}

	// Priority 3: main field
	if pkg.Main != "" {
		return "node " + pkg.Main, 3000
	}

	// Fallback
	return "node index.js", 3000
}

func mergeDeps(pkg *packageJSON) map[string]string {
	all := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for k, v := range pkg.Dependencies {
		all[k] = v
	}
	for k, v := range pkg.DevDependencies {
		all[k] = v
	}
	return all
}

func nodeStartMain(pkg *packageJSON) string {
	if pkg.Main != "" {
		return "node " + pkg.Main
	}
	return "node index.js"
}

func hasServerDep(deps map[string]string) bool {
	for _, d := range []string{"express", "fastify", "hono", "koa", "@hono/node-server"} {
		if _, ok := deps[d]; ok {
			return true
		}
	}
	return false
}

var portFlagRe = regexp.MustCompile(`(?:-p|--port)\s+(\d+)`)

func parsePortFromCmd(cmd string) int {
	m := portFlagRe.FindStringSubmatch(cmd)
	if len(m) < 2 {
		return 0
	}
	port, _ := strconv.Atoi(m[1])
	return port
}

// Suppress unused import warning.
var _ = fmt.Sprintf
