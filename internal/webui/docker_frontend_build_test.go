package webui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frontendLocaleImports are JSON locale paths imported from frontend/src/i18n (repo-root relative).
var frontendLocaleImports = []string{
	"locales/en.json",
	"locales/de.json",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func TestFrontendLocaleCatalogPathsExist(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range frontendLocaleImports {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("locale catalog %s: %v", rel, err)
		}
	}
}

func TestFrontendPackageManagerPinned(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "frontend", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pkg.PackageManager, "pnpm@") {
		t.Fatalf("frontend packageManager must pin pnpm (got %q)", pkg.PackageManager)
	}
}

func TestDockerfileCopiesLocalesBeforeFrontendBuild(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	var sawLocalesCopy bool
	var sawFrontendBuild bool
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "COPY locales/en.json") || strings.HasPrefix(trimmed, "COPY locales ./locales") {
			sawLocalesCopy = true
		}
		if strings.Contains(trimmed, "pnpm --dir frontend build") {
			sawFrontendBuild = true
			if !sawLocalesCopy {
				t.Fatal("Dockerfile runs frontend build before COPY locales; i18n imports need repo locales/")
			}
			return
		}
	}
	if !sawFrontendBuild {
		t.Fatal("Dockerfile: frontend build RUN line not found")
	}
}

func TestDockerfileVerifiesLocaleFilesBeforeFrontendBuild(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "test -s locales/en.json") || !strings.Contains(s, "test -s locales/de.json") {
		t.Fatal("Dockerfile must verify locale JSON files exist and are non-empty before frontend build")
	}
	if !strings.Contains(s, "COPY locales/en.json") {
		t.Fatal("Dockerfile must copy locale JSON files explicitly (empty locales/ dir passes COPY but breaks tsc)")
	}
}

func TestDockerfileUsesPinnedPnpmForCorepack(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "corepack prepare pnpm@10.33.0") {
		t.Fatal("Dockerfile must pin corepack pnpm@10.33.0 (Dockhand blocks npm registry lookups for other versions)")
	}
}

func TestDockerfileSingleRunFrontendBuild(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	frontendIdx := strings.Index(s, "AS frontend-builder")
	if frontendIdx < 0 {
		t.Fatal("Dockerfile: frontend-builder stage not found")
	}
	builderStage := s[frontendIdx:]
	nextFrom := strings.Index(builderStage[1:], "\nFROM ")
	if nextFrom >= 0 {
		builderStage = builderStage[:nextFrom+1]
	}
	buildCount := strings.Count(builderStage, "pnpm --dir frontend build")
	if buildCount != 1 {
		t.Fatalf("frontend-builder must run pnpm build exactly once in a single layer (got %d); split RUN steps break pnpm on some builders", buildCount)
	}
	if strings.Contains(builderStage, "pnpm --dir frontend build") &&
		!strings.Contains(builderStage, "pnpm --dir frontend install") {
		t.Fatal("frontend-builder RUN must install deps before build")
	}
}
