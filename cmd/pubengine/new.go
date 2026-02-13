package main

import (
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
	ProjectName string
	ModuleName  string
	SiteName    string
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

	// Build template data.
	data := scaffoldData{
		ProjectName: dirName,
		ModuleName:  name,
		SiteName:    toTitle(dirName),
	}

	fmt.Printf("Creating new pubengine project: %s\n\n", dirName)

	root := "templates"

	err := fs.WalkDir(scaffold.Templates, root, func(path string, d fs.DirEntry, err error) error {
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

		// Rename dotenv to .env.example.
		if filepath.Base(outPath) == "dotenv" {
			outPath = filepath.Join(filepath.Dir(outPath), ".env.example")
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

		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		defer f.Close()

		if err := tmpl.Execute(f, data); err != nil {
			return fmt.Errorf("execute template %s: %w", path, err)
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
	fmt.Println("  npm install")
	fmt.Println("  make run")
	fmt.Println()
	fmt.Printf("Edit views/*.templ to customize your templates, then run 'make templ'.\n")
	fmt.Printf("Set ADMIN_PASSWORD and ADMIN_SESSION_SECRET in .env for production.\n")
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
