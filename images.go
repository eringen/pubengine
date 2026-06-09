package pubengine

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
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
	maxImageWidth     = 800
	maxImageDimension = 10000
	maxImagePixels    = 24_000_000
	jpegQuality       = 80
	maxUploadSize     = 10 << 20 // 10MB
	uploadsSubdir     = "uploads"
)

// processImage decodes an image from src, optionally resizes it to maxImageWidth,
// and encodes it as JPEG. Returns metadata and the encoded bytes.
func processImage(src io.Reader, originalName string) (Image, []byte, error) {
	raw, err := io.ReadAll(io.LimitReader(src, maxUploadSize+1))
	if err != nil {
		return Image{}, nil, err
	}
	if len(raw) > maxUploadSize {
		return Image{}, nil, fmt.Errorf("%w: file exceeds 10 MB", ErrInvalidImage)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return Image{}, nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return Image{}, nil, fmt.Errorf("%w: image dimensions are too large", ErrInvalidImage)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return Image{}, nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// Resize if wider than max
	if w > maxImageWidth {
		newH := max(1, h*maxImageWidth/w)
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
	slug := Slugify(base)
	if slug == "" {
		return "image"
	}
	if len(slug) > 80 {
		slug = strings.TrimRight(slug[:80], "-")
	}
	return slug
}

// ErrInvalidImage identifies malformed or unsupported input, rather than storage errors.
var ErrInvalidImage = errors.New("invalid image")

// storeImage exclusively creates a new file and removes it if metadata persistence fails.
func (a *App) storeImage(img Image, data []byte) error {
	dir := filepath.Join(a.staticDir, uploadsSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	suffix := make([]byte, 16)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	img.Filename = slugifyFilename(img.OriginalName) + "-" + hex.EncodeToString(suffix) + ".jpg"
	path := filepath.Join(dir, img.Filename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	if err := a.Store.SaveImage(img); err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
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
		if errors.Is(err, ErrInvalidImage) {
			return c.String(http.StatusBadRequest, err.Error())
		}
		return err
	}

	if err := a.storeImage(img, data); err != nil {
		return err
	}

	return a.renderImageList(c)
}

func (a *App) handleImageDelete(c echo.Context) error {
	if !IsAdmin(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/")
	}

	if err := a.deleteImage(c.Param("filename")); err != nil {
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

func (a *App) deleteImage(filename string) error {
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\%") || !strings.HasSuffix(filename, ".jpg") {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid image filename")
	}
	// Serialize the reversible filesystem/metadata transition within this app.
	a.imageMu.Lock()
	defer a.imageMu.Unlock()
	var exists int
	if err := a.Store.db.QueryRow("SELECT 1 FROM images WHERE filename=?", filename).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return echo.ErrNotFound
		}
		return err
	}
	path := filepath.Join(a.staticDir, uploadsSubdir, filename)
	// Keep the bytes in a private temporary directory until metadata deletion succeeds.
	trash, err := os.MkdirTemp(filepath.Dir(path), ".delete-")
	if err != nil {
		return err
	}
	defer os.Remove(trash)
	backup := filepath.Join(trash, filename)
	moved := true
	if err := os.Rename(path, backup); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		moved = false
	}
	if err := a.Store.DeleteImage(filename); err != nil {
		if moved {
			return errors.Join(err, os.Rename(backup, path))
		}
		return err
	}
	if moved {
		return os.Remove(backup)
	}
	return nil
}
