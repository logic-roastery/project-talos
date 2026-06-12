package detect

// StaticProvider detects plain HTML/CSS/JS projects with no build system.
type StaticProvider struct{}

func (p *StaticProvider) Name() string { return "static" }

func (p *StaticProvider) Detect(root string) bool {
	// Must have index.html and no build system files
	if !fileExists(root, "index.html") {
		return false
	}
	// Exclude if any build system is present
	for _, f := range []string{"package.json", "go.mod", "pom.xml", "build.gradle", "build.gradle.kts"} {
		if fileExists(root, f) {
			return false
		}
	}
	return true
}

func (p *StaticProvider) Plan(root string) (*BuildPlan, error) {
	return &BuildPlan{
		Provider:  "static",
		Runtime:   "nginx:alpine",
		Port:      80,
		StaticDir: ".",
	}, nil
}
