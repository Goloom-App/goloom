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
	if !strings.Contains(string(dockerfile), "packageManager.split('@')[1]") {
		t.Fatal("Dockerfile must read pnpm version from frontend/package.json packageManager (avoid hardcoded drift)")
	}
}
