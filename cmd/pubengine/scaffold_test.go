package main

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/eringen/pubengine/scaffold"
)

// Exercise actual generated code, including templates that ordinary go test never compiles.
func TestGeneratedSite(t *testing.T) {
	generator := "github.com/a-h/templ/cmd/templ@v0.3.1020"
	dir := t.TempDir()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	data := scaffoldData{ProjectName: "audit", ModuleName: "example.com/audit", SiteName: "Audit", AdminPassword: "unique-password", SessionSecret: strings.Repeat("k", 43)}
	err = fs.WalkDir(scaffold.Templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "templates")
		target := filepath.Join(dir, strings.TrimSuffix(rel, ".tmpl"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		src, err := scaffold.Templates.ReadFile(path)
		if err != nil {
			return err
		}
		tmpl, err := template.New(path).Parse(string(src))
		if err != nil {
			return err
		}
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		defer f.Close()
		return tmpl.Execute(f, data)
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("PUBENGINE_TEST_PUBLISHED") != "1" {
		mod := "module example.com/audit\n\ngo 1.26.0\n\nrequire github.com/eringen/pubengine v0.0.0\nreplace github.com/eringen/pubengine => " + root + "\n"
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "views", "render_test.go"), []byte(generatedViewTest), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for _, args := range [][]string{{"go", "run", generator, "generate", "-path", dir}, {"go", "mod", "tidy"}, {"go", "test", "./..."}} {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	if os.Getenv("PUBENGINE_BROWSER_TESTS") == "1" {
		for _, args := range [][]string{{"npm", "install", "--no-audit", "--no-fund"}, {"make", "css", "js"}, {"go", "build", "-o", "site", "."}, {"node", filepath.Join(root, "scripts", "browser_test.cjs"), dir}} {
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, output)
			} else {
				t.Logf("%s", output)
			}
		}
	}
}

const generatedViewTest = `package views
import("bytes";"context";"encoding/json";"strings";"testing";"github.com/eringen/pubengine";"golang.org/x/net/html")
func TestStructuredData(t *testing.T){
 name:="A </script> title"
 var b bytes.Buffer
 if err:=JsonLD(pubengine.WebsiteJsonLD(pubengine.SiteConfig{Name:name,URL:"https://example.com"})).Render(context.Background(),&b);err!=nil{t.Fatal(err)}
 doc,err:=html.Parse(strings.NewReader(b.String()));if err!=nil{t.Fatal(err)}
 count:=0
 var walk func(*html.Node);walk=func(n *html.Node){if n.Type==html.ElementNode&&n.Data=="script"{count++;if n.FirstChild==nil{t.Fatal("empty JSON")};var value map[string]any;if err:=json.Unmarshal([]byte(n.FirstChild.Data),&value);err!=nil{t.Fatal(err)};if value["name"]!=name||value["@type"]!="WebSite"{t.Fatal(value)}};for c:=n.FirstChild;c!=nil;c=c.NextSibling{walk(c)}};walk(doc);if count!=1{t.Fatal(count)}
}
func TestEditorKeepsRevisionAndErrors(t *testing.T){var b bytes.Buffer;p:=pubengine.BlogPost{OriginalSlug:"old",Slug:"new",Revision:3,Content:"unsaved text",Error:"Conflict"};if err:=AdminFormPartial(p,"csrf").Render(context.Background(),&b);err!=nil{t.Fatal(err)};for _,want:=range []string{"original_slug","revision","unsaved text","Conflict"}{if !strings.Contains(b.String(),want){t.Fatal(want)}}}
func TestRuntimeMetadata(t *testing.T){
 cfg:=pubengine.SiteConfig{Name:"Runtime name",URL:"https://runtime.test",Description:"Runtime description",Author:"Runtime author"}
 views:=New(cfg); p:=pubengine.BlogPost{Title:"Post",Slug:"post",Summary:"Summary",Content:"Body"}
 var full,partial bytes.Buffer
 if err:=views.Post(p,nil,cfg.URL).Render(context.Background(),&full);err!=nil{t.Fatal(err)}
 if err:=views.PostPartial(p,nil,cfg.URL).Render(context.Background(),&partial);err!=nil{t.Fatal(err)}
 if full.String()!=partial.String(){t.Fatal("partial metadata differs")}
 for _,want:=range []string{cfg.Name,cfg.Author,"https://runtime.test/blog/post/"}{if !strings.Contains(full.String(),want){t.Fatal(want)}}
}
`
