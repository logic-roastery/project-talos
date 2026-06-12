package detect

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// JavaProvider detects Java/Maven/Gradle projects.
type JavaProvider struct{}

func (p *JavaProvider) Name() string { return "java" }

func (p *JavaProvider) Detect(root string) bool {
	return fileExists(root, "pom.xml") || fileExists(root, "build.gradle") || fileExists(root, "build.gradle.kts")
}

func (p *JavaProvider) Plan(root string) (*BuildPlan, error) {
	plan := &BuildPlan{
		Provider: "java",
		Port:     8080,
	}

	if fileExists(root, "pom.xml") {
		// Maven project
		javaVer := detectJavaVersion(root, "pom.xml")
		plan.Runtime = "eclipse-temurin:" + javaVer + "-jdk-alpine"
		plan.Install = []string{"mvn dependency:go-offline -B"}
		plan.Build = []string{"mvn package -DskipTests -B"}
		plan.StartCommand = "java -jar app.jar"
		plan.OutputDir = "target/*.jar"
	} else {
		// Gradle project
		gradleFile := "build.gradle"
		if fileExists(root, "build.gradle.kts") {
			gradleFile = "build.gradle.kts"
		}
		javaVer := detectJavaVersion(root, gradleFile)
		plan.Runtime = "eclipse-temurin:" + javaVer + "-jdk-alpine"
		plan.Install = []string{"gradle dependencies || true"}
		plan.Build = []string{"gradle build -x test"}
		plan.StartCommand = "java -jar app.jar"
		plan.OutputDir = "build/libs/*.jar"
	}

	return plan, nil
}

var (
	javaVersionRe     = regexp.MustCompile(`<java\.version>(\d+)</java\.version>`)
	compilerSourceRe  = regexp.MustCompile(`<maven\.compiler\.source>(\d+)</maven\.compiler\.source>`)
	gradleSourceRe    = regexp.MustCompile(`sourceCompatibility\s*=\s*['"]?(\d+)`)
	gradleToolchainRe = regexp.MustCompile(`languageVersion\s*=\s*['"]?(\d+)`)
)

// detectJavaVersion reads a build file and extracts the Java version.
// Returns "21" as default if not found.
func detectJavaVersion(root, filename string) string {
	f, err := os.Open(root + "/" + filename)
	if err != nil {
		return "21"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasSuffix(filename, ".xml") {
			// Maven
			if m := javaVersionRe.FindStringSubmatch(line); len(m) > 1 {
				return m[1]
			}
			if m := compilerSourceRe.FindStringSubmatch(line); len(m) > 1 {
				return m[1]
			}
		} else {
			// Gradle
			if m := gradleSourceRe.FindStringSubmatch(line); len(m) > 1 {
				return m[1]
			}
			if m := gradleToolchainRe.FindStringSubmatch(line); len(m) > 1 {
				return m[1]
			}
		}
	}

	return "21"
}
