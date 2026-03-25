package stego

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
)

// toNRGBA converts any image to *image.NRGBA so we can manipulate raw
// (non-premultiplied) pixel bytes directly, avoiding precision loss from
// premultiplied-alpha conversions that destroy LSBs.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

// pixelRGB reads R, G, B bytes directly from the NRGBA pixel buffer.
func pixelRGB(img *image.NRGBA, x, y int) (r, g, b byte) {
	off := img.PixOffset(x, y)
	return img.Pix[off], img.Pix[off+1], img.Pix[off+2]
}

// setPixelRGB writes R, G, B bytes directly into the NRGBA pixel buffer.
func setPixelRGB(img *image.NRGBA, x, y int, r, g, b byte) {
	off := img.PixOffset(x, y)
	img.Pix[off] = r
	img.Pix[off+1] = g
	img.Pix[off+2] = b
}

// HideData hides data in an image using LSB steganography
// Returns the modified image as PNG bytes
func HideData(imagePath string, data []byte) ([]byte, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return hideDataInImage(img, data)
}

// isUsablePixel returns true if the pixel at (x,y) is fully opaque (A==255).
// Only fully opaque pixels are used for LSB steganography. Transparent (A==0)
// and semi-transparent (0<A<255) pixels are left untouched to preserve the
// visual appearance of images with transparency and anti-aliased edges.
func isUsablePixel(img *image.NRGBA, x, y int) bool {
	off := img.PixOffset(x, y)
	return img.Pix[off+3] == 255
}

// countUsablePixels counts pixels with A == 255 (fully opaque).
func countUsablePixels(img *image.NRGBA) int {
	count := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] == 255 {
			count++
		}
	}
	return count
}

func hideDataInImage(img image.Image, data []byte) ([]byte, error) {
	// Convert to NRGBA to work with raw bytes (no premultiplication)
	nrgba := toNRGBA(img)
	bounds := nrgba.Bounds()

	// Only count fully opaque pixels (A==255) as usable for hiding data.
	// Transparent and semi-transparent pixels are left untouched to
	// preserve the visual appearance (transparency + anti-aliased edges).
	usable := countUsablePixels(nrgba)
	maxBits := usable * 3 // 3 channels (RGB) per usable pixel
	totalBits := 32 + len(data)*8

	if totalBits > maxBits {
		return nil, errors.New("image too small to hide data: need larger image")
	}

	// Prepend data length (4 bytes, big endian)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	payload := append(lenBuf, data...)

	bitIndex := 0
	totalPayloadBits := len(payload) * 8

	for y := bounds.Min.Y; y < bounds.Max.Y && bitIndex < totalPayloadBits; y++ {
		for x := bounds.Min.X; x < bounds.Max.X && bitIndex < totalPayloadBits; x++ {
			// Skip non-fully-opaque pixels — transparent and semi-transparent
			// pixels must remain untouched to preserve visual appearance.
			if !isUsablePixel(nrgba, x, y) {
				continue
			}

			r, g, b := pixelRGB(nrgba, x, y)

			// Modify R channel
			if bitIndex < totalPayloadBits {
				r = setLSBByte(r, getBit(payload, bitIndex))
				bitIndex++
			}

			// Modify G channel
			if bitIndex < totalPayloadBits {
				g = setLSBByte(g, getBit(payload, bitIndex))
				bitIndex++
			}

			// Modify B channel
			if bitIndex < totalPayloadBits {
				b = setLSBByte(b, getBit(payload, bitIndex))
				bitIndex++
			}

			setPixelRGB(nrgba, x, y, r, g, b)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ExtractData extracts hidden data from a steganographic image
func ExtractData(imagePath string) ([]byte, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return extractDataFromImage(img)
}

func extractDataFromImage(img image.Image) ([]byte, error) {
	// Convert to NRGBA to read raw bytes (no premultiplication)
	nrgba := toNRGBA(img)
	bounds := nrgba.Bounds()

	// Collect LSBs only from fully opaque pixels (matching hide logic).
	usable := countUsablePixels(nrgba)
	totalBits := usable * 3
	bits := make([]byte, 0, totalBits)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if !isUsablePixel(nrgba, x, y) {
				continue
			}
			r, g, b := pixelRGB(nrgba, x, y)
			bits = append(bits, r&1, g&1, b&1)
		}
	}

	if len(bits) < 32 {
		return nil, errors.New("image too small to contain hidden data")
	}

	// First 32 bits = payload length (big endian uint32)
	lenBuf := make([]byte, 4)
	for i := 0; i < 32; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if bits[i] == 1 {
			lenBuf[byteIdx] |= 1 << bitIdx
		}
	}

	dataLen := binary.BigEndian.Uint32(lenBuf)
	if dataLen == 0 || dataLen > 50*1024*1024 { // max 50 MB
		return nil, errors.New("no hidden data found or corrupted")
	}

	needed := 32 + int(dataLen)*8
	if len(bits) < needed {
		return nil, errors.New("image does not contain enough data")
	}

	// Next dataLen*8 bits = actual payload
	data := make([]byte, dataLen)
	for i := 0; i < int(dataLen)*8; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if bits[32+i] == 1 {
			data[byteIdx] |= 1 << bitIdx
		}
	}

	return data, nil
}

// HideDataInBytes works with raw image bytes instead of file path
func HideDataInBytes(imgData []byte, data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, err
	}
	return hideDataInImage(img, data)
}

func getBit(data []byte, index int) byte {
	byteIndex := index / 8
	bitIndex := 7 - (index % 8)
	return (data[byteIndex] >> bitIndex) & 1
}

func setLSBByte(val byte, bit byte) byte {
	return (val & 0xFE) | (bit & 1)
}

// Keep old helpers for backwards compatibility if needed elsewhere
func setBit(data []byte, index int, val uint32) {
	byteIndex := index / 8
	bitIndex := 7 - (index % 8)
	if val == 1 {
		data[byteIndex] |= 1 << bitIndex
	} else {
		data[byteIndex] &^= 1 << bitIndex
	}
}

func getLSB(val uint32) uint32 {
	return (val >> 8) & 1 // RGBA returns 16-bit values
}

func setLSB(val uint32, bit uint32) uint32 {
	val = val >> 8
	if bit == 1 {
		val |= 1
	} else {
		val &^= 1
	}
	return val << 8
}
