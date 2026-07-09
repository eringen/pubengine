package pubengine

import (
	"context"
	"github.com/a-h/templ"
	"io"
)

func editorPage(form templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Edit post</title><link rel="stylesheet" href="/public/tailwind.css"></head><body><main><a href="/admin/">Back to posts</a>`); err != nil {
			return err
		}
		if err := form.Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</main></body></html>`)
		return err
	})
}
