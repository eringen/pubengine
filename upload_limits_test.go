package pubengine

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadAdmissionAndCancellation(t *testing.T) {
	a := testHTTPApp(t)
	for i := 0; i < cap(a.uploadSlots); i++ {
		a.uploadSlots <- struct{}{}
	}
	w := httptest.NewRecorder()
	a.Echo.ServeHTTP(w, httptest.NewRequest("POST", "/admin/images/upload/", strings.NewReader("body")))
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code, w.Body)
	}
	for len(a.uploadSlots) > 0 {
		<-a.uploadSlots
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := processImageContext(ctx, strings.NewReader("invalid"), "test.png"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/admin/images/upload/", strings.NewReader("bad multipart"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	a.Echo.ServeHTTP(w, r)
	if len(a.uploadSlots) != 0 {
		t.Fatal("upload slot leaked")
	}
}

func BenchmarkImageProcessing(b *testing.B) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 2400, 1600))); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := processImage(bytes.NewReader(input.Bytes()), "photo.png"); err != nil {
			b.Fatal(err)
		}
	}
}
