package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/eringen/pubengine/scaffold"
)

func TestScaffoldCredentialsAreIndependent(t *testing.T) {
	a, err := randomSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := randomSecret()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 32 || len(b) < 32 {
		t.Fatal("invalid random credentials")
	}
	src, err := scaffold.Templates.ReadFile("templates/dotenv.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New("env").Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, scaffoldData{AdminPassword: a, SessionSecret: b}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), []byte("changeme")) || !bytes.Contains(out.Bytes(), []byte("ADMIN_SESSION_SECRET="+b)) {
		t.Fatal("invalid scaffold credentials")
	}
	path := filepath.Join(t.TempDir(), ".env.example")
	if err := writeEnvExample(path, tmpl, scaffoldData{AdminPassword: a, SessionSecret: b}); err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(example, []byte(a)) || bytes.Contains(example, []byte(b)) {
		t.Fatal("example contains credentials")
	}
}

func TestNewProjectKeepsCredentialsPrivate(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	var previous []byte
	for _, name := range []string{"first-site", "second-site"} {
		if err := runNew("example.com/" + name); err != nil {
			t.Fatal(err)
		}
		private, err := os.ReadFile(filepath.Join(name, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(name, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatal("credentials are not private")
		}
		example, err := os.ReadFile(filepath.Join(name, ".env.example"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(example, []byte("ADMIN_SESSION_SECRET=\n")) || !bytes.Contains(example, []byte("ADMIN_PASSWORD=\n")) {
			t.Fatal("example includes a credential")
		}
		if bytes.Equal(private, example) || bytes.Equal(private, previous) {
			t.Fatal("credentials were not generated independently")
		}
		ignore, err := os.ReadFile(filepath.Join(name, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(ignore, []byte("\n.env\n")) {
			t.Fatal("private credentials are not ignored")
		}
		previous = private
	}
}
