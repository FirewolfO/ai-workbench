package filetools

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestCatalogAlwaysExposesPureGoTools(t *testing.T) {
	catalog := New().Catalog()
	available := map[string]bool{}
	for _, tool := range catalog {
		available[tool.ID] = tool.Available
		if tool.ID == "pdf_to_word" && len(tool.requires) != 0 {
			t.Fatal("internal requirements must not be returned to clients")
		}
	}
	if !available["images_to_pdf"] || !available["zip_files"] {
		t.Fatalf("pure Go tools should always be available: %#v", available)
	}
}

func TestZipFilesSanitizesAndDeduplicatesNames(t *testing.T) {
	result, err := New().Run(context.Background(), "zip_files", []Input{
		{Name: "../工作计划.txt", Data: []byte("first")},
		{Name: "工作计划.txt", Data: []byte("second")},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(result.Data), int64(len(result.Data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 || reader.File[0].Name != "工作计划.txt" || reader.File[1].Name != "工作计划-2.txt" {
		t.Fatalf("unexpected archive entries: %#v", reader.File)
	}
}

func TestImagesToPDFProducesDownloadablePDF(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 20, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 20; x++ {
			canvas.Set(x, y, color.RGBA{R: 23, G: 107, B: 85, A: 255})
		}
	}
	var source bytes.Buffer
	if err := png.Encode(&source, canvas); err != nil {
		t.Fatal(err)
	}
	result, err := New().Run(context.Background(), "images_to_pdf", []Input{{Name: "photo.png", ContentType: "image/png", Data: source.Bytes()}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "application/pdf" || !bytes.HasPrefix(result.Data, []byte("%PDF")) {
		t.Fatalf("unexpected PDF result: type=%s prefix=%q", result.ContentType, result.Data[:4])
	}
}

func TestRejectsMismatchedExtensionAndTooFewFiles(t *testing.T) {
	service := New()
	if _, err := service.Run(context.Background(), "images_to_pdf", []Input{{Name: "notes.txt", Data: []byte("not an image")}}, Options{}); err != ErrInvalid {
		t.Fatalf("expected invalid extension, got %v", err)
	}
	if _, err := service.Run(context.Background(), "merge_pdf", []Input{{Name: "one.pdf", Data: []byte("%PDF")}}, Options{}); err != ErrInvalid {
		t.Fatalf("expected minimum file count validation, got %v", err)
	}
}
