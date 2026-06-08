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
		if strings.HasPrefix(trimmed, "COPY locales") {
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

func TestDockerfileUsesPackageManagerPnpmVersion(t *testing.T) {
	root := repoRoot(t)
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	pkgJSON, err := os.ReadFile(filepath.Join(root, "frontend", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(pkgJSON, &pkg); err != nil {
		t.Fatal(err)
	}
	content := string(dockerfile)
	if !strings.Contains(content, "packageManager.split('@')[1]") {
		t.Fatal("Dockerfile must read pnpm version from frontend/package.json packageManager (avoid hardcoded drift)")
	}
}

func TestDockerfileVerifiesLocalesBeforeFrontendBuild(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "test -f locales/en.json") || !strings.Contains(s, "test -f locales/de.json") {
		t.Fatal("Dockerfile must verify locale catalogs exist before frontend build (tsc imports ../../../locales/*.json)")
	}
}

func TestDockerfilePnpmCorepackFallback(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if !strings.Contains(s, "corepack prepare failed") {
		t.Fatal("Dockerfile must fall back when corepack prepare fails (restricted npm registry in CI/Dockhand)")
	}
	if !strings.Contains(s, "pnpm-linux-") {
		t.Fatal("Dockerfile must download pnpm from GitHub releases on corepack fallback")
	}
}

func TestDockerfileFrontendBuilderUsesBash(t *testing.T) {
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
	if !strings.Contains(builderStage, `SHELL ["/bin/bash"`) {
		t.Fatal("Dockerfile frontend-builder must use bash (dash /bin/sh lacks pipefail and breaks RUN scripts)")
	}
}
