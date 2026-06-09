package pubengine

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestImageDimensionLimits(t *testing.T) {
	for _, size := range []image.Point{{1000, 1}, {10001, 1}, {1, 10001}} {
		var src bytes.Buffer
		if err := png.Encode(&src, image.NewRGBA(image.Rect(0, 0, size.X, size.Y))); err != nil {
			t.Fatal(err)
		}
		img, _, err := processImage(&src, "wide.png")
		if size.X > maxImageDimension || size.Y > maxImageDimension {
			if !errors.Is(err, ErrInvalidImage) {
				t.Fatal(err)
			}
		} else if err != nil || img.Width != 800 || img.Height != 1 {
			t.Fatalf("%+v %v", img, err)
		}
	}
}

func TestConcurrentImageUploadsAndRollback(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "images.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(SiteConfig{}, ViewFuncs{}, WithStaticDir(t.TempDir()))
	a.Store = s
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := a.storeImage(Image{OriginalName: "same.png"}, []byte{byte(i)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	images, err := s.ListImages()
	if err != nil || len(images) != 10 {
		t.Fatalf("%d %v", len(images), err)
	}
	seen := map[byte]bool{}
	for _, img := range images {
		b, err := os.ReadFile(filepath.Join(a.staticDir, uploadsSubdir, img.Filename))
		if err != nil {
			t.Fatal(err)
		}
		seen[b[0]] = true
	}
	if len(seen) != 10 {
		t.Fatal("image overwritten")
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_image_insert BEFORE INSERT ON images BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := a.storeImage(Image{OriginalName: "same.png"}, []byte("failed")); err == nil {
		t.Fatal("expected insert failure")
	}
	files, err := os.ReadDir(filepath.Join(a.staticDir, uploadsSubdir))
	if err != nil || len(files) != 10 {
		t.Fatal("orphan file", len(files), err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_image_delete BEFORE DELETE ON images BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := a.deleteImage(images[0].Filename); err == nil {
		t.Fatal("expected delete failure")
	}
	if _, err := os.Stat(filepath.Join(a.staticDir, uploadsSubdir, images[0].Filename)); err != nil {
		t.Fatal("image not restored", err)
	}
}

func TestDeleteImageValidationAndMissingFile(t *testing.T) {
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(SiteConfig{}, ViewFuncs{}, WithStaticDir(t.TempDir()))
	a.Store = s
	if err := os.MkdirAll(filepath.Join(a.staticDir, uploadsSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "../a.jpg", "..%2Fa.jpg", "a.png"} {
		if err := a.deleteImage(name); err == nil {
			t.Fatal("invalid filename accepted", name)
		}
	}
	if err := s.SaveImage(Image{Filename: "missing.jpg"}); err != nil {
		t.Fatal(err)
	}
	if err := a.deleteImage("missing.jpg"); err != nil {
		t.Fatal(err)
	}
}
