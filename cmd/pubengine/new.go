package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/eringen/pubengine/scaffold"
)

// scaffoldData holds the template variables passed to every scaffold template.
type scaffoldData struct {
	ProjectName   string
	ModuleName    string
	SiteName      string
	AdminPassword string
	SessionSecret string
}

func runNew(name string) error {
	// Derive project directory name from the last path segment.
	dirName := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		dirName = name[idx+1:]
	}

	// Check if directory already exists.
	if _, err := os.Stat(dirName); err == nil {
		return fmt.Errorf("directory %q already exists", dirName)
	}

	// Give every new project independent credentials.
	password, err := randomSecret()
	if err != nil {
		return err
	}
	secret, err := randomSecret()
	if err != nil {
		return err
	}

	// Build template data.
	data := scaffoldData{
		ProjectName:   dirName,
		ModuleName:    name,
		SiteName:      toTitle(dirName),
		AdminPassword: password,
		SessionSecret: secret,
	}

	fmt.Printf("Creating new pubengine project: %s\n\n", dirName)

	root := "templates"

	err = fs.WalkDir(scaffold.Templates, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute the relative path from the template root.
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// Compute the output path, stripping the .tmpl suffix.
		outPath := filepath.Join(dirName, relPath)
		outPath = strings.TrimSuffix(outPath, ".tmpl")

		// Rename dotfiles (embed.FS cannot store files starting with ".").
		switch filepath.Base(outPath) {
		case "dotenv":
			outPath = filepath.Join(filepath.Dir(outPath), ".env.example")
		case "dotgitignore":
			outPath = filepath.Join(filepath.Dir(outPath), ".gitignore")
		}

		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}

		// Read the template file.
		content, err := scaffold.Templates.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		// Parse and execute as a Go text/template.
		tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", path, err)
		}

		// Ensure parent directory exists.
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}

		mode := os.FileMode(0o644)
		if filepath.Base(outPath) == ".env.example" {
			mode = 0o600
		}
		f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}

		if err := tmpl.Execute(f, data); err != nil {
			f.Close()
			os.Remove(outPath)
			return fmt.Errorf("execute template %s: %w", path, err)
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", outPath, err)
		}

		fmt.Printf("  created %s\n", outPath)
		return nil
	})
	if err != nil {
		return err
	}

	// Resolve dependencies and generate go.sum.
	fmt.Println("\nResolving Go dependencies...")
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dirName
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nWarning: go mod tidy failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'cd %s && go mod tidy' manually after fixing.\n", dirName)
	}

	fmt.Println()
	fmt.Println("Done! Next steps:")
	fmt.Println()
	fmt.Printf("  cd %s\n", dirName)
	fmt.Println("  cp .env.example .env")
	fmt.Println("  npm install")
	fmt.Println("  make run")
	fmt.Println()
	fmt.Printf("Edit views/*.templ to customize your templates, then run 'make templ'.\n")
	fmt.Printf("Update ADMIN_PASSWORD and ADMIN_SESSION_SECRET in .env before deploying.\n")
	return nil
}

// toTitle converts a hyphenated or lowercase name to a title-case string.
// e.g. "my-blog" -> "My Blog", "myblog" -> "Myblog"
func toTitle(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
