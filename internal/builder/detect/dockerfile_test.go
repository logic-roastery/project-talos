package detect

import (
	"strings"
	"testing"
)

func TestGenerateDockerfile(t *testing.T) {
	tests := []struct {
		name     string
		plan     *BuildPlan
		contains []string
	}{
		{
			name: "static",
			plan: &BuildPlan{Provider: "static", Runtime: "nginx:alpine", Port: 80, StaticDir: "."},
			contains: []string{
				"FROM nginx:alpine",
				"EXPOSE 80",
				"/usr/share/nginx/html",
			},
		},
		{
			name: "node npm no build",
			plan: &BuildPlan{
				Provider:     "node",
				Runtime:      "node:22-alpine",
				Install:      []string{"npm ci"},
				StartCommand: "npm start",
				Port:         3000,
			},
			contains: []string{
				"FROM node:22-alpine",
				"RUN npm ci",
				"EXPOSE 3000",
				`CMD ["npm", "start"]`,
			},
		},
		{
			name: "node npm with build",
			plan: &BuildPlan{
				Provider:     "node",
				Runtime:      "node:22-alpine",
				Install:      []string{"npm ci"},
				Build:        []string{"npm run build"},
				StartCommand: "npm start",
				Port:         3000,
			},
			contains: []string{
				"FROM node:22-alpine AS builder",
				"RUN npm run build",
				"npm ci --omit=dev",
				`CMD ["npm", "start"]`,
			},
		},
		{
			name: "node bun",
			plan: &BuildPlan{
				Provider:     "node",
				Runtime:      "oven/bun:1-alpine",
				Install:      []string{"bun install --frozen-lockfile"},
				StartCommand: "bun run start",
				Port:         3000,
			},
			contains: []string{
				"FROM oven/bun:1-alpine",
				"bun install --frozen-lockfile",
				`CMD ["bun", "run", "start"]`,
			},
		},
		{
			name: "go",
			plan: &BuildPlan{
				Provider:    "go",
				Runtime:     "golang:1.25-alpine",
				BinaryName:  "myapp",
				Port:        8080,
				BuildTarget: "./cmd/app",
			},
			contains: []string{
				"FROM golang:1.25-alpine AS builder",
				"CGO_ENABLED=0",
				"go build",
				"./cmd/app",
				"FROM alpine:3.23",
				"ca-certificates",
				"EXPOSE 8080",
				`CMD ["./myapp"]`,
			},
		},
		{
			name: "java maven",
			plan: &BuildPlan{
				Provider:     "java",
				Runtime:      "eclipse-temurin:21-jdk-alpine",
				Install:      []string{"mvn dependency:go-offline -B"},
				Build:        []string{"mvn package -DskipTests -B"},
				StartCommand: "java -jar app.jar",
				Port:         8080,
				OutputDir:    "target/*.jar",
			},
			contains: []string{
				"FROM eclipse-temurin:21-jdk-alpine AS builder",
				"mvn dependency:go-offline",
				"mvn package -DskipTests",
				"FROM eclipse-temurin:21-jre-alpine",
				"EXPOSE 8080",
				`CMD ["java", "-jar", "app.jar"]`,
			},
		},
		{
			name: "java gradle",
			plan: &BuildPlan{
				Provider:     "java",
				Runtime:      "eclipse-temurin:21-jdk-alpine",
				Install:      []string{"gradle dependencies || true"},
				Build:        []string{"gradle build -x test"},
				StartCommand: "java -jar app.jar",
				Port:         8080,
				OutputDir:    "build/libs/*.jar",
			},
			contains: []string{
				"FROM eclipse-temurin:21-jdk-alpine AS builder",
				"gradle build -x test",
				"FROM eclipse-temurin:21-jre-alpine",
				"EXPOSE 8080",
				`CMD ["java", "-jar", "app.jar"]`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(GenerateDockerfile(tt.plan))
			if got == "" {
				t.Fatal("GenerateDockerfile returned empty")
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Dockerfile missing %q\n\nGot:\n%s", want, got)
				}
			}
		})
	}
}

func TestCmdToJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"npm start", `["npm", "start"]`},
		{"node index.js", `["node", "index.js"]`},
		{"bun run start", `["bun", "run", "start"]`},
		{"npx next start", `["npx", "next", "start"]`},
		{"", `["sh"]`},
		{"./server", `["./server"]`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cmdToJSON(tt.input)
			if got != tt.want {
				t.Errorf("cmdToJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateDockerfileUnknownProvider(t *testing.T) {
	got := GenerateDockerfile(&BuildPlan{Provider: "unknown"})
	if got != nil {
		t.Errorf("expected nil for unknown provider, got %q", string(got))
	}
}
