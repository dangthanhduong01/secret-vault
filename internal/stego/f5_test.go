package stego

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestF5RoundTrip(t *testing.T) {
	// Create a test PNG image (100x100, solid colour)
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_f5.png")

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

	// Hide data using F5
	secret := []byte("Hello, F5 Steganography! This is a secret message for testing.")
	result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	// Save result
	outPath := filepath.Join(tmpDir, "output_f5.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	// Extract data
	extracted, err := ExtractDataF5(outPath)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
	t.Log("✓ F5 round-trip succeeded")
}

func TestF5RoundTripNRGBA(t *testing.T) {
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_f5_nrgba.png")

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

	secret := []byte("Testing F5 with NRGBA image format!")
	result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_f5_nrgba.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	extracted, err := ExtractDataF5(outPath)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
	t.Log("✓ F5 NRGBA round-trip succeeded")
}

func TestF5SemiTransparent(t *testing.T) {
	// Mix of opaque and semi-transparent pixels.
	// Only opaque pixels (A=255) should be used.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_f5_alpha.png")

	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if y < 80 {
				img.Set(x, y, color.NRGBA{R: 200, G: 150, B: 100, A: 255})
			} else {
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

	secret := []byte("F5 with alpha channel test!")
	result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_f5_alpha.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	extracted, err := ExtractDataF5(outPath)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}

	// Verify semi-transparent pixels are preserved
	f2, _ := os.Open(outPath)
	decoded, _, _ := image.Decode(f2)
	f2.Close()
	nrgba := decoded.(*image.NRGBA)
	semiCount := 0
	for i := 3; i < len(nrgba.Pix); i += 4 {
		if nrgba.Pix[i] == 128 {
			semiCount++
		}
	}
	if semiCount != 100*20 { // 20 rows × 100 cols of A=128
		t.Errorf("Semi-transparent pixel count: got %d, want %d", semiCount, 100*20)
	}
	t.Log("✓ F5 semi-transparent round-trip succeeded")
}

func TestF5TransparentBackground(t *testing.T) {
	// PNG with transparent background, opaque circle in center.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "f5_transparent_bg.png")

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

	secret := []byte("F5 hidden in transparent PNG!")
	result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_f5_transparent.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify transparency preserved
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
	if transparentCount == 0 {
		t.Fatal("All pixels became opaque — transparency was destroyed!")
	}
	t.Logf("Transparent pixels preserved: %d", transparentCount)

	// Extract and verify
	extracted, err := ExtractDataF5(outPath)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}
	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
	t.Log("✓ F5 transparent background round-trip succeeded")
}

func TestF5FewerChanges(t *testing.T) {
	// Verify that F5 matrix encoding changes fewer pixels than naive LSB.
	// For random data, F5 (1,3,2) should change about 37.5% of groups
	// (3 out of 4 syndromes require a flip, but only 1 flip per group of 3).
	// Naive LSB changes about 50% of all values.
	const size = 200
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 123, G: 200, B: 77, A: 255})
		}
	}

	// Record original LSBs
	origLSBs := make([]byte, size*size*3)
	for i := range origLSBs {
		ch := i % 3 // R=0, G=1, B=2
		px := i / 3
		off := px * 4
		origLSBs[i] = img.Pix[off+ch] & 1
	}

	// Large payload to exercise embedding fully
	payload := make([]byte, 2000)
	for i := range payload {
		payload[i] = byte(i * 37)
	}

	// F5 hide
	f5Img, err := hideDataF5InImage(img, payload)
	if err != nil {
		t.Fatalf("hideDataF5InImage failed: %v", err)
	}
	decoded, _, _ := image.Decode(bytes.NewReader(f5Img))
	f5NRGBA := toNRGBA(decoded)

	// Count changes
	f5Changes := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			off := f5NRGBA.PixOffset(x, y)
			origIdx := (y*size + x) * 3
			if (f5NRGBA.Pix[off] & 1) != origLSBs[origIdx] {
				f5Changes++
			}
			if (f5NRGBA.Pix[off+1] & 1) != origLSBs[origIdx+1] {
				f5Changes++
			}
			if (f5NRGBA.Pix[off+2] & 1) != origLSBs[origIdx+2] {
				f5Changes++
			}
		}
	}

	// Naive LSB would change approximately 50% of the used LSBs.
	// F5 with (1,3,2) should change at most 1 per group of 3,
	// and on average about 37.5% of groups → ~12.5% of LSBs used.
	totalMsgBits := (4 + len(payload)) * 8
	groupsNeeded := (totalMsgBits + f5K - 1) / f5K
	lsbsUsed := groupsNeeded * f5N

	t.Logf("F5 changes: %d / %d LSBs used (%.1f%%)", f5Changes, lsbsUsed, float64(f5Changes)*100/float64(lsbsUsed))
	t.Logf("Naive LSB would change ~%.0f / %d LSBs (50%%)", float64(lsbsUsed)*0.5, lsbsUsed)

	// F5 should change at most groupsNeeded LSBs (1 per group).
	if f5Changes > groupsNeeded {
		t.Errorf("F5 changed %d LSBs but has only %d groups — at most %d changes expected", f5Changes, groupsNeeded, groupsNeeded)
	}

	// F5 should change significantly fewer than naive 50%.
	naiveExpected := float64(lsbsUsed) * 0.5
	if float64(f5Changes) >= naiveExpected {
		t.Errorf("F5 changed %d LSBs, not fewer than naive estimate %.0f", f5Changes, naiveExpected)
	}

	t.Log("✓ F5 matrix encoding changes fewer pixels than naive LSB")
}

func TestF5MatrixEncodingUnit(t *testing.T) {
	// Unit-test the (1,3,2) matrix encoding and extraction directly.
	// Embed known 2-bit messages into known cover groups and verify.

	tests := []struct {
		cover  [3]byte // cover LSBs (only bit 0 matters)
		m0, m1 byte    // message bits
	}{
		{[3]byte{0, 0, 0}, 0, 0},
		{[3]byte{0, 0, 0}, 0, 1},
		{[3]byte{0, 0, 0}, 1, 0},
		{[3]byte{0, 0, 0}, 1, 1},
		{[3]byte{1, 1, 1}, 0, 0},
		{[3]byte{1, 1, 1}, 0, 1},
		{[3]byte{1, 1, 1}, 1, 0},
		{[3]byte{1, 1, 1}, 1, 1},
		{[3]byte{1, 0, 1}, 0, 0},
		{[3]byte{0, 1, 0}, 1, 1},
	}

	for i, tc := range tests {
		lsbs := []byte{tc.cover[0] & 1, tc.cover[1] & 1, tc.cover[2] & 1}
		// Build a 1-byte payload where first 2 bits = m0, m1
		payload := []byte{(tc.m0 << 7) | (tc.m1 << 6)}
		embedF5(lsbs, payload, 2)

		// Extract
		bits := extractF5Bits(lsbs, 2)
		if bits[0] != tc.m0 || bits[1] != tc.m1 {
			t.Errorf("Case %d: cover=%v msg=(%d,%d) → extracted=(%d,%d)",
				i, tc.cover, tc.m0, tc.m1, bits[0], bits[1])
		}

		// Verify at most 1 change
		changes := 0
		for j := 0; j < 3; j++ {
			if lsbs[j] != (tc.cover[j] & 1) {
				changes++
			}
		}
		if changes > 1 {
			t.Errorf("Case %d: matrix encoding changed %d LSBs (max 1 allowed)", i, changes)
		}
	}
	t.Log("✓ All matrix encoding unit tests passed")
}

func TestF5LargePayload(t *testing.T) {
	// Test with a larger payload to exercise many groups.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_f5_large.png")

	img := image.NewNRGBA(image.Rect(0, 0, 500, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 500; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: byte((x * 7) % 256),
				G: byte((y * 13) % 256),
				B: byte((x + y) % 256),
				A: 255,
			})
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

	// ~50 KB payload
	secret := make([]byte, 50000)
	for i := range secret {
		secret[i] = byte(i % 251) // quasi-random
	}

	result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output_f5_large.png")
	if err := os.WriteFile(outPath, result, 0644); err != nil {
		t.Fatal(err)
	}

	extracted, err := ExtractDataF5(outPath)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}

	if !bytes.Equal(extracted, secret) {
		t.Fatalf("Large payload mismatch! len(expected)=%d, len(got)=%d", len(secret), len(extracted))
	}
	t.Log("✓ F5 large payload (50 KB) round-trip succeeded")
}

func TestF5ImageTooSmall(t *testing.T) {
	// Tiny image that can't hold the data.
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "tiny_f5.png")

	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 100, G: 100, B: 100, A: 255})
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

	bigPayload := make([]byte, 100)
	_, err = HideDataF5(imgPath, bigPayload)
	if err == nil {
		t.Fatal("Expected error for image too small, got nil")
	}
	t.Logf("✓ Got expected error: %v", err)
}

func TestF5HideDataInBytes(t *testing.T) {
	// Test HideDataF5InBytes with raw PNG bytes.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 42, G: 128, B: 200, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	secret := []byte("F5 from bytes!")
	result, err := HideDataF5InBytes(buf.Bytes(), secret)
	if err != nil {
		t.Fatalf("HideDataF5InBytes failed: %v", err)
	}

	// Extract from the result bytes directly
	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractDataF5FromImage(decoded)
	if err != nil {
		t.Fatalf("extractDataF5FromImage failed: %v", err)
	}

	if string(extracted) != string(secret) {
		t.Fatalf("Mismatch!\n  Expected: %q\n  Got:      %q", secret, extracted)
	}
	t.Log("✓ F5 HideDataInBytes round-trip succeeded")
}

func TestF5AndLSBIndependent(t *testing.T) {
	// Verify F5 and classic LSB produce different outputs
	// (different encoding schemes should not be interchangeable).
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test_independence.png")

	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 150, B: 100, A: 255})
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

	secret := []byte("Test independence of F5 and LSB")

	lsbResult, err := HideData(imgPath, secret)
	if err != nil {
		t.Fatalf("HideData (LSB) failed: %v", err)
	}

	f5Result, err := HideDataF5(imgPath, secret)
	if err != nil {
		t.Fatalf("HideDataF5 failed: %v", err)
	}

	if bytes.Equal(lsbResult, f5Result) {
		t.Fatal("F5 and LSB produced identical outputs — they should differ!")
	}

	// Each can extract its own data
	lsbOut := filepath.Join(tmpDir, "lsb_out.png")
	os.WriteFile(lsbOut, lsbResult, 0644)
	lsbExtracted, err := ExtractData(lsbOut)
	if err != nil {
		t.Fatalf("ExtractData (LSB) failed: %v", err)
	}
	if string(lsbExtracted) != string(secret) {
		t.Fatalf("LSB extraction mismatch")
	}

	f5Out := filepath.Join(tmpDir, "f5_out.png")
	os.WriteFile(f5Out, f5Result, 0644)
	f5Extracted, err := ExtractDataF5(f5Out)
	if err != nil {
		t.Fatalf("ExtractDataF5 failed: %v", err)
	}
	if string(f5Extracted) != string(secret) {
		t.Fatalf("F5 extraction mismatch")
	}

	t.Log("✓ F5 and LSB are independent: different outputs, each self-consistent")
}
