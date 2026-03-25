package stego

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	// Create a test PNG image (100x100, solid red)
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 150, B: 100, A: 255})
		}
	}

	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Hide data
	secret := []byte("Hello, Steganography! This is a secret message for testing.")
	result, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData failed: %v", err)
	}

	// Save result
	outPath := filepath.Join(tmpDir, "output.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	// Log what image type we get back
	f2, _ := os.Open(outPath)
	decoded, format, _ := image.Decode(f2)
	f2.Close()
	t.Logf("Decoded image type: %T, format: %s", decoded, format)

	// Extract data
	extracted, err := ExtractData(outPath)
	if err != nil {
		t.Fatalf("ExtractData failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
}

func TestRoundTripNRGBA(t *testing.T) {
	// Create NRGBA image (this is what Go's PNG encoder often produces on decode)
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_nrgba.png")

	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 150, B: 100, A: 255})
		}
	}

	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	secret := []byte("Testing NRGBA round trip!")
	result, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_nrgba.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	f2, _ := os.Open(outPath)
	decoded, format, _ := image.Decode(f2)
	f2.Close()
	t.Logf("Decoded image type: %T, format: %s", decoded, format)

	extracted, err := ExtractData(outPath)
	if err != nil {
		t.Fatalf("ExtractData failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
}

func TestRoundTripSemiTransparent(t *testing.T) {
	// Image with a mix of fully opaque and semi-transparent pixels.
	// Only the fully opaque pixels (A=255) should be used for hiding.
	// Semi-transparent edge pixels must be preserved for visual quality.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_alpha.png")

	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if y < 80 {
				// Fully opaque region
				img.Set(x, y, color.NRGBA{R: 200, G: 150, B: 100, A: 255})
			} else {
				// Semi-transparent edge (like anti-aliasing)
				img.Set(x, y, color.NRGBA{R: 200, G: 150, B: 100, A: 128})
			}
		}
	}

	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	secret := []byte("Alpha channel test")
	result, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_alpha.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	extracted, err := ExtractData(outPath)
	if err != nil {
		t.Fatalf("ExtractData failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
}

func TestRoundTripTransparentBackground(t *testing.T) {
	// Simulate a PNG with a transparent background (like "free ship.png").
	// Only the centre circle is opaque; the rest is fully transparent (A=0).
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "transparent_bg.png")

	const size = 200
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0 // all transparent
	}
	cx, cy, radius := size/2, size/2, 60
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= radius*radius {
				img.SetNRGBA(x, y, color.NRGBA{R: 80, G: 180, B: 120, A: 255})
			}
		}
	}

	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	secret := []byte("Hidden in transparent PNG!")
	result, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_transparent.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify transparent pixels are still transparent
	f2, _ := os.Open(outPath)
	decoded, _, _ := image.Decode(f2)
	f2.Close()
	nrgba := decoded.(*image.NRGBA)
	transparentCount := 0
	for i := 3; i < len(nrgba.Pix); i += 4 {
		if nrgba.Pix[i] == 0 {
			transparentCount++
		}
	}
	t.Logf("Transparent pixels preserved: %d", transparentCount)
	if transparentCount == 0 {
		t.Fatal("All pixels became opaque — transparency was destroyed!")
	}

	// Verify data extraction
	extracted, err := ExtractData(outPath)
	if err != nil {
		t.Fatalf("ExtractData failed: %v", err)
	}
	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
}

func TestRoundTripRealTransparentPNG(t *testing.T) {
	// Test with the actual "free ship.png" if it exists
	imgPath := "../../free ship.png"
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		t.Skip("free ship.png not found, skipping real-file test")
	}

	// Read original image stats
	fOrig, _ := os.Open(imgPath)
	origImg, _, _ := image.Decode(fOrig)
	fOrig.Close()
	origNRGBA := origImg.(*image.NRGBA)
	origTransparent, origSemi, origOpaque := 0, 0, 0
	for i := 3; i < len(origNRGBA.Pix); i += 4 {
		a := origNRGBA.Pix[i]
		if a == 0 {
			origTransparent++
		} else if a == 255 {
			origOpaque++
		} else {
			origSemi++
		}
	}
	t.Logf("Original — Transparent: %d, Semi: %d, Opaque: %d", origTransparent, origSemi, origOpaque)

	secret := []byte("Steganography test with real transparent PNG!")
	result, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData failed: %v", err)
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "freeship_stego.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	// Check transparency + semi-transparency preserved
	f, _ := os.Open(outPath)
	decoded, _, _ := image.Decode(f)
	f.Close()
	nrgba := decoded.(*image.NRGBA)
	outTransparent, outSemi, outOpaque := 0, 0, 0
	for i := 3; i < len(nrgba.Pix); i += 4 {
		a := nrgba.Pix[i]
		if a == 0 {
			outTransparent++
		} else if a == 255 {
			outOpaque++
		} else {
			outSemi++
		}
	}
	t.Logf("Output   — Transparent: %d, Semi: %d, Opaque: %d", outTransparent, outSemi, outOpaque)

	if outTransparent != origTransparent {
		t.Errorf("Transparent count changed: %d → %d", origTransparent, outTransparent)
	}
	if outSemi != origSemi {
		t.Errorf("Semi-transparent count changed: %d → %d (edges destroyed!)", origSemi, outSemi)
	}
	if outTransparent == 0 {
		t.Fatal("All pixels became opaque — transparency destroyed!")
	}

	// Verify extraction
	extracted, err := ExtractData(outPath)
	if err != nil {
		t.Fatalf("ExtractData failed: %v", err)
	}
	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
	t.Log("✓ Real transparent PNG: data hidden & extracted, transparency + edges preserved")
}
