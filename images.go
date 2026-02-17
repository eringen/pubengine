package pubengine

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/image/draw"
)

const (
	maxImageWidth  = 800
	jpegQuality    = 80
	maxUploadSize  = 10 << 20 // 10MB
	uploadsSubdir  = "uploads"
)

// processImage decodes an image from src, optionally resizes it to maxImageWidth,
// and encodes it as JPEG. Returns metadata and the encoded bytes.
func processImage(src io.Reader, originalName string) (Image, []byte, error) {
	img, _, err := image.Decode(src)
	if err != nil {
		return Image{}, nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Resize if wider than max
	if w > maxImageWidth {
		newH := h * maxImageWidth / w
		dst := image.NewRGBA(image.Rect(0, 0, maxImageWidth, newH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
		w = maxImageWidth
		h = newH
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Image{}, nil, fmt.Errorf("encode jpeg: %w", err)
	}

	filename := slugifyFilename(originalName) + ".jpg"

	return Image{
		Filename:     filename,
		OriginalName: originalName,
		Width:        w,
		Height:       h,
		Size:         buf.Len(),
		UploadedAt:   time.Now().UTC().Format(time.RFC3339),
	}, buf.Bytes(), nil
}

// slugifyFilename converts a filename (without extension) to a URL-safe slug.
func slugifyFilename(name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return Slugify(base)
}

// ensureUniqueFilename appends a counter if filename already exists in the directory or database.
func (a *App) ensureUniqueFilename(img *Image) {
	dir := filepath.Join(a.staticDir, uploadsSubdir)
	base := strings.TrimSuffix(img.Filename, ".jpg")
	candidate := img.Filename
	counter := 1
	for {
		// Check filesystem
		if _, err := os.Stat(filepath.Join(dir, candidate)); err == nil {
			counter++
			candidate = fmt.Sprintf("%s-%d.jpg", base, counter)
			continue
		}
		// Check database
		existing, _ := a.Store.ListImages()
		found := false
		for _, ex := range existing {
			if ex.Filename == candidate {
				found = true
				break
			}
		}
		if found {
			counter++
			candidate = fmt.Sprintf("%s-%d.jpg", base, counter)
			continue
		}
		break
	}
	img.Filename = candidate
}

func (a *App) handleImageUpload(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}

	file, err := c.FormFile("image")
	if err != nil {
		return c.String(http.StatusBadRequest, "No image file provided")
	}
	if file.Size > maxUploadSize {
		return c.String(http.StatusBadRequest, "File too large (max 10MB)")
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	img, data, err := processImage(src, file.Filename)
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid image: "+err.Error())
	}

	a.ensureUniqueFilename(&img)

	// Ensure uploads directory exists
	dir := filepath.Join(a.staticDir, uploadsSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create uploads dir: %w", err)
	}

	// Write file
	if err := os.WriteFile(filepath.Join(dir, img.Filename), data, 0o644); err != nil {
		return fmt.Errorf("write image: %w", err)
	}

	// Save metadata
	if err := a.Store.SaveImage(img); err != nil {
		return err
	}

	return a.renderImageList(c)
}

func (a *App) handleImageDelete(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}

	filename := c.Param("filename")
	if filename == "" {
		return c.String(http.StatusBadRequest, "Filename required")
	}

	// Delete from filesystem
	path := filepath.Join(a.staticDir, uploadsSubdir, filename)
	_ = os.Remove(path) // ignore error if file already gone

	// Delete from database
	if err := a.Store.DeleteImage(filename); err != nil {
		return err
	}

	return a.renderImageList(c)
}

func (a *App) handleImageList(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}
	return a.renderImageList(c)
}

func (a *App) renderImageList(c echo.Context) error {
	images, err := a.Store.ListImages()
	if err != nil {
		return err
	}
	return Render(c, a.Views.AdminImages(images, CsrfToken(c)))
}
