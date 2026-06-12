package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, root string)
		wantProv   string
		wantPort   int
		wantErr    bool
		wantErrStr string
	}{
		{
			name: "static detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "index.html", "<html></html>")
			},
			wantProv: "static",
			wantPort: 80,
		},
		{
			name: "node npm detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"scripts":{"start":"node index.js"}}`)
				writeFile(t, root, "package-lock.json", "{}")
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node bun detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"scripts":{"start":"bun run index.ts"}}`)
				writeFile(t, root, "bun.lockb", "")
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node next detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"dependencies":{"next":"14.0.0"}}`)
				writeFile(t, root, "package-lock.json", "{}")
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node build and start",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"scripts":{"build":"tsc","start":"node dist/index.js"}}`)
				writeFile(t, root, "package-lock.json", "{}")
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node pnpm detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"scripts":{"start":"pnpm start"}}`)
				writeFile(t, root, "pnpm-lock.yaml", "")
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node angular detected",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"dependencies":{"@angular/core":"17.0.0"}}`)
				writeFile(t, root, "package-lock.json", "{}")
			},
			wantProv: "node",
			wantPort: 4200,
		},
		{
			name: "go root main",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "go.mod", "module github.com/user/myapp\n\ngo 1.25\n")
				writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
			},
			wantProv: "go",
			wantPort: 8080,
		},
		{
			name: "go cmd layout",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "go.mod", "module github.com/user/myapp\n\ngo 1.25\n")
				os.MkdirAll(filepath.Join(root, "cmd", "app"), 0755)
				writeFile(t, root, "cmd/app/main.go", "package main\n\nfunc main() {}\n")
			},
			wantProv: "go",
			wantPort: 8080,
		},
		{
			name: "go no main",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "go.mod", "module github.com/user/mylib\n\ngo 1.25\n")
				writeFile(t, root, "lib.go", "package lib\n\nfunc Hello() {}\n")
			},
			wantProv: "go",
			wantPort: 8080,
		},
		{
			name: "java maven",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "pom.xml", `<project><properties><java.version>17</java.version></properties></project>`)
			},
			wantProv: "java",
			wantPort: 8080,
		},
		{
			name: "java gradle",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "build.gradle", `plugins { id 'java' } sourceCompatibility = '17'`)
			},
			wantProv: "java",
			wantPort: 8080,
		},
		{
			name:       "no match",
			setup:      func(t *testing.T, root string) {},
			wantErr:    true,
			wantErrStr: "unsupported project type",
		},
		{
			name: "static excluded when package.json present",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "index.html", "<html></html>")
				writeFile(t, root, "package.json", `{"scripts":{"start":"node index.js"}}`)
			},
			wantProv: "node",
			wantPort: 3000,
		},
		{
			name: "node port from start script",
			setup: func(t *testing.T, root string) {
				writeFile(t, root, "package.json", `{"scripts":{"start":"node server.js --port 8888"}}`)
				writeFile(t, root, "package-lock.json", "{}")
			},
			wantProv: "node",
			wantPort: 8888,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)

			plan, err := Detect(root)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrStr != "" && !containsStr(err.Error(), tt.wantErrStr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrStr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plan.Provider != tt.wantProv {
				t.Errorf("provider = %q, want %q", plan.Provider, tt.wantProv)
			}
			if plan.Port != tt.wantPort {
				t.Errorf("port = %d, want %d", plan.Port, tt.wantPort)
			}
		})
	}
}

func TestGoMainDetection(t *testing.T) {
	t.Run("cmd matches module name", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "go.mod", "module github.com/user/myapp\n\ngo 1.25\n")
		os.MkdirAll(filepath.Join(root, "cmd", "myapp"), 0755)
		os.MkdirAll(filepath.Join(root, "cmd", "other"), 0755)
		writeFile(t, root, "cmd/myapp/main.go", "package main\n\nfunc main() {}\n")
		writeFile(t, root, "cmd/other/main.go", "package main\n\nfunc main() {}\n")

		plan, err := Detect(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plan.BuildTarget != "./cmd/myapp" {
			t.Errorf("build target = %q, want %q", plan.BuildTarget, "./cmd/myapp")
		}
	})

	t.Run("multiple cmd dirs picks first alphabetically", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "go.mod", "module github.com/user/app\n\ngo 1.25\n")
		os.MkdirAll(filepath.Join(root, "cmd", "zebra"), 0755)
		os.MkdirAll(filepath.Join(root, "cmd", "alpha"), 0755)
		writeFile(t, root, "cmd/zebra/main.go", "package main\n\nfunc main() {}\n")
		writeFile(t, root, "cmd/alpha/main.go", "package main\n\nfunc main() {}\n")

		plan, err := Detect(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plan.BuildTarget != "./cmd/alpha" {
			t.Errorf("build target = %q, want %q", plan.BuildTarget, "./cmd/alpha")
		}
	})

	t.Run("binary name from module", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "go.mod", "module github.com/user/myserver\n\ngo 1.25\n")
		writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")

		plan, err := Detect(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plan.BinaryName != "myserver" {
			t.Errorf("binary name = %q, want %q", plan.BinaryName, "myserver")
		}
	})
}

func TestJavaVersionDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		isXML   bool
	}{
		{
			name:    "maven java.version",
			content: `<project><properties><java.version>17</java.version></properties></project>`,
			want:    "17",
			isXML:   true,
		},
		{
			name:    "maven compiler source",
			content: `<project><properties><maven.compiler.source>21</maven.compiler.source></properties></project>`,
			want:    "21",
			isXML:   true,
		},
		{
			name:    "gradle sourceCompatibility",
			content: `sourceCompatibility = '17'`,
			want:    "17",
		},
		{
			name:    "gradle toolchain",
			content: `languageVersion = '21'`,
			want:    "21",
		},
		{
			name:    "default",
			content: `<project></project>`,
			want:    "21",
			isXML:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			filename := "build.gradle"
			if tt.isXML {
				filename = "pom.xml"
			}
			writeFile(t, root, filename, tt.content)

			got := detectJavaVersion(root, filename)
			if got != tt.want {
				t.Errorf("detectJavaVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
