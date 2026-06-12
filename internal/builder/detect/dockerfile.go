package detect

import (
	"bytes"
	"strings"
	"text/template"
)

// cmdToJSON converts a shell command string to a Docker exec-form JSON array.
// e.g. "npm start" → ["npm", "start"]
func cmdToJSON(cmd string) string {
	if cmd == "" {
		return `["sh"]`
	}
	parts := strings.Fields(cmd)
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = `"` + p + `"`
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// GenerateDockerfile produces a Dockerfile from a BuildPlan.
func GenerateDockerfile(plan *BuildPlan) []byte {
	funcMap := template.FuncMap{
		"cmd": cmdToJSON,
	}

	var tmplStr string
	switch plan.Provider {
	case "static":
		tmplStr = staticDockerfileTmpl
	case "node":
		if plan.Runtime == "oven/bun:1-alpine" {
			tmplStr = bunDockerfileTmpl
		} else if len(plan.Build) > 0 {
			tmplStr = nodeBuildDockerfileTmpl
		} else {
			tmplStr = nodeDockerfileTmpl
		}
	case "go":
		tmplStr = goDockerfileTmpl
	case "java":
		if len(plan.Install) > 0 && plan.Install[0] == "mvn dependency:go-offline -B" {
			tmplStr = mavenDockerfileTmpl
		} else {
			tmplStr = gradleDockerfileTmpl
		}
	default:
		return nil
	}

	tmpl := template.Must(template.New("dockerfile").Funcs(funcMap).Parse(tmplStr))
	var buf bytes.Buffer
	tmpl.Execute(&buf, plan)
	return buf.Bytes()
}

const staticDockerfileTmpl = `FROM nginx:alpine
COPY . /usr/share/nginx/html
EXPOSE {{.Port}}
`

const nodeDockerfileTmpl = `FROM {{.Runtime}}
WORKDIR /app
COPY package*.json ./
RUN {{index .Install 0}}
COPY . .
EXPOSE {{.Port}}
CMD {{cmd .StartCommand}}
`

const nodeBuildDockerfileTmpl = `FROM {{.Runtime}} AS builder
WORKDIR /app
COPY package*.json ./
RUN {{index .Install 0}}
COPY . .
RUN {{index .Build 0}}

FROM {{.Runtime}}
WORKDIR /app
COPY package*.json ./
{{- if eq .Runtime "node:22-alpine"}}
RUN npm ci --omit=dev
{{- end}}
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/.output ./.output
COPY --from=builder /app/build ./build
EXPOSE {{.Port}}
CMD {{cmd .StartCommand}}
`

const bunDockerfileTmpl = `FROM oven/bun:1-alpine AS builder
WORKDIR /app
COPY bun.lockb package.json ./
RUN bun install --frozen-lockfile
COPY . .
{{- if .Build}}
RUN {{index .Build 0}}
{{- end}}

FROM oven/bun:1-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE {{.Port}}
CMD {{cmd .StartCommand}}
`

const goDockerfileTmpl = `FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /{{.BinaryName}} {{.BuildTarget}}

FROM alpine:3.23
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /{{.BinaryName}} .
EXPOSE {{.Port}}
CMD ["./{{.BinaryName}}"]
`

const mavenDockerfileTmpl = `FROM {{.Runtime}} AS builder
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline -B
COPY src ./src
RUN mvn package -DskipTests -B

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar
EXPOSE {{.Port}}
CMD ["java", "-jar", "app.jar"]
`

const gradleDockerfileTmpl = `FROM {{.Runtime}} AS builder
WORKDIR /app
COPY build.gradle* gradle* ./
COPY gradle ./gradle
RUN gradle dependencies || true
COPY src ./src
RUN gradle build -x test

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/build/libs/*.jar app.jar
EXPOSE {{.Port}}
CMD ["java", "-jar", "app.jar"]
`
