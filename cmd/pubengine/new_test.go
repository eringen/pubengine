package main

import (
	"bytes"
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
}
