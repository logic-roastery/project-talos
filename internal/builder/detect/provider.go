package detect

// BuildPlan holds the resolved configuration for building a project.
type BuildPlan struct {
	Provider     string   // "node", "go", "java", "static"
	Runtime      string   // e.g. "node:22-alpine", "golang:1.25-alpine"
	Install      []string // install commands (run in build stage)
	Build        []string // build commands (run after install)
	StartCommand string   // CMD instruction
	Port         int      // EXPOSE / default port
	OutputDir    string   // for compiled apps: binary/jar output path in build stage
	BinaryName   string   // for Go: output binary name
	StaticDir    string   // for static: directory to copy to nginx
	BuildTarget  string   // for Go: build target path (e.g. "./cmd/app" or ".")
}

// Provider detects whether a project matches and returns a BuildPlan.
type Provider interface {
	Name() string
	Detect(root string) bool
	Plan(root string) (*BuildPlan, error)
}
