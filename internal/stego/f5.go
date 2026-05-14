package stego

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"math/rand"
	"os"
)

// F5 Steganography

// f5Seed is the fixed seed used when no password is provided.
// Payload is already AES-256-GCM encrypted,
// The permutation is a second layer of obscurity
var f5Seed = []byte("SecretVault-F5-Permutation-Seed")

// ── Matrix encoding parameters ──────────────────────────────────────
// k = 2, n = 2^k − 1 = 3
// Embed 2 message bits per group of 3 cover LSBs, changing at most 1.
const (
	f5K = 2 // bits embedded per group
	f5N = 3 // group size (2^k − 1)
)

// HideDataF5 hides data in an image using the F5 algorithm.
// Returns the modified image as PNG bytes.
func HideDataF5(imagePath string, data []byte) ([]byte, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return hideDataF5InImage(img, data)
}

// ExtractDataF5 extracts hidden data from an F5-steganographic image.
func ExtractDataF5(imagePath string) ([]byte, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	return extractDataF5FromImage(img)
}

// HideDataF5InBytes works with raw image bytes instead of a file path.
func HideDataF5InBytes(imgData []byte, data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, err
	}
	return hideDataF5InImage(img, data)
}

// hideDataF5InImage embeds data into img using F5 matrix encoding with permutative straddling.
func hideDataF5InImage(img image.Image, data []byte) ([]byte, error) {
	nrgba := toNRGBA(img)

	// Collect indices of all usable LSB positions (3 per opaque pixel: R, G, B).
	indices := usableLSBIndices(nrgba)
	totalLSBs := len(indices)

	// Build payload: [4-byte big-endian length][data]
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	payload := append(lenBuf, data...)

	totalMsgBits := len(payload) * 8
	// Number of groups needed: ceil(totalMsgBits / k)
	groupsNeeded := (totalMsgBits + f5K - 1) / f5K
	lsbsNeeded := groupsNeeded * f5N

	if lsbsNeeded > totalLSBs {
		return nil, errors.New("image too small to hide data (F5): need larger image")
	}

	// Permute the indices using a deterministic PRNG.
	perm := permuteIndices(indices, f5Seed)

	// Read current LSB values into a flat slice.
	lsbs := readLSBValues(nrgba, perm)

	// Embed message bits using (1, 3, 2) matrix encoding.
	embedF5(lsbs, payload, totalMsgBits)

	// Write modified LSBs back into the image.
	writeLSBValues(nrgba, perm, lsbs)

	var buf bytes.Buffer
	if err := png.Encode(&buf, nrgba); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// extractDataF5FromImage extracts F5-encoded data from img.
func extractDataF5FromImage(img image.Image) ([]byte, error) {
	nrgba := toNRGBA(img)

	indices := usableLSBIndices(nrgba)
	perm := permuteIndices(indices, f5Seed)
	lsbs := readLSBValues(nrgba, perm)

	// We need at least 32 message bits (4-byte length) = ceil(32/2)*3 = 48 LSBs.
	if len(lsbs) < 48 {
		return nil, errors.New("image too small to contain F5 hidden data")
	}

	// Extract length (first 32 message bits).
	lenBits := extractF5Bits(lsbs, 32)
	lenBuf := bitsToBytes(lenBits, 4)
	dataLen := binary.BigEndian.Uint32(lenBuf)

	if dataLen == 0 || dataLen > 50*1024*1024 {
		return nil, errors.New("no F5 hidden data found or corrupted")
	}

	totalMsgBits := 32 + int(dataLen)*8
	groupsNeeded := (totalMsgBits + f5K - 1) / f5K
	lsbsNeeded := groupsNeeded * f5N

	if lsbsNeeded > len(lsbs) {
		return nil, errors.New("image does not contain enough F5 data")
	}

	// Extract all message bits (length + data).
	allBits := extractF5Bits(lsbs, totalMsgBits)

	// Skip the first 32 bits (length), convert rest to bytes.
	dataBits := allBits[32:]
	result := bitsToBytes(dataBits, int(dataLen))

	return result, nil
}

// lsbIndex represents a single LSB position in the image:
// the pixel offset in img.Pix and the channel (0=R, 1=G, 2=B).
type lsbIndex struct {
	pixOffset int // offset into img.Pix (multiple of 4)
	channel   int // 0, 1, or 2
}

// usableLSBIndices returns the list of all LSB positions in fully-opaque
// pixels (A==255), in row-major order, 3 entries per pixel (R, G, B).
func usableLSBIndices(img *image.NRGBA) []lsbIndex {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	out := make([]lsbIndex, 0, w*h*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			off := img.PixOffset(x, y)
			if img.Pix[off+3] != 255 {
				continue // skip non-opaque
			}
			out = append(out,
				lsbIndex{off, 0}, // R
				lsbIndex{off, 1}, // G
				lsbIndex{off, 2}, // B
			)
		}
	}
	return out
}

// permuteIndices returns a permuted copy of indices using a PRNG seeded
// from SHA-256(seed). This is the "permutative straddling" step of F5.
func permuteIndices(indices []lsbIndex, seed []byte) []lsbIndex {
	h := sha256.Sum256(seed)
	// Use first 8 bytes of hash as int64 seed for the PRNG.
	s := int64(binary.BigEndian.Uint64(h[:8]))
	rng := rand.New(rand.NewSource(s))

	perm := make([]lsbIndex, len(indices))
	copy(perm, indices)

	// Fisher-Yates shuffle
	for i := len(perm) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}
	return perm
}

// readLSBValues reads the LSB of each position listed in perm.
func readLSBValues(img *image.NRGBA, perm []lsbIndex) []byte {
	out := make([]byte, len(perm))
	for i, idx := range perm {
		out[i] = img.Pix[idx.pixOffset+idx.channel] & 1
	}
	return out
}

// writeLSBValues writes LSBs back into img at the positions listed in perm.
func writeLSBValues(img *image.NRGBA, perm []lsbIndex, lsbs []byte) {
	for i, idx := range perm {
		off := idx.pixOffset + idx.channel
		img.Pix[off] = (img.Pix[off] & 0xFE) | (lsbs[i] & 1)
	}
}

// embedF5 modifies lsbs[] in place so that the (1,3,2) matrix encoding
// carries the message bits from payload.
func embedF5(lsbs []byte, payload []byte, totalBits int) {
	gi := 0 // group index in lsbs[]
	for bi := 0; bi < totalBits; bi += f5K {
		// Read 2 message bits (pad with 0 if at the end).
		m0 := getMsgBit(payload, bi)
		m1 := byte(0)
		if bi+1 < totalBits {
			m1 = getMsgBit(payload, bi+1)
		}

		base := gi * f5N
		c0 := lsbs[base]
		c1 := lsbs[base+1]
		c2 := lsbs[base+2]

		// Syndrome: s = H · c (mod 2)
		s0 := (c1 ^ c2) & 1 // row 0 of H: (0,1,1)
		s1 := (c0 ^ c2) & 1 // row 1 of H: (1,0,1)

		// Diff
		d0 := s0 ^ m0
		d1 := s1 ^ m1

		// Flip at most one LSB
		switch {
		case d0 == 0 && d1 == 0:
			// no change
		case d0 == 0 && d1 == 1:
			lsbs[base] ^= 1 // flip c0
		case d0 == 1 && d1 == 0:
			lsbs[base+1] ^= 1 // flip c1
		case d0 == 1 && d1 == 1:
			lsbs[base+2] ^= 1 // flip c2
		}

		gi++
	}
}

// extractF5Bits extracts totalMsgBits message bits from lsbs[] using
// the (1,3,2) Hamming extraction (syndrome computation).
func extractF5Bits(lsbs []byte, totalMsgBits int) []byte {
	bits := make([]byte, totalMsgBits)
	gi := 0
	for bi := 0; bi < totalMsgBits; bi += f5K {
		base := gi * f5N
		c0 := lsbs[base]
		c1 := lsbs[base+1]
		c2 := lsbs[base+2]

		// Syndrome = H · c (mod 2)
		bits[bi] = (c1 ^ c2) & 1 // m0
		if bi+1 < totalMsgBits {
			bits[bi+1] = (c0 ^ c2) & 1 // m1
		}

		gi++
	}
	return bits
}

// Bit utilities

// getMsgBit returns bit at position idx (MSB-first within each byte).
func getMsgBit(data []byte, idx int) byte {
	byteIdx := idx / 8
	bitIdx := 7 - (idx % 8)
	return (data[byteIdx] >> bitIdx) & 1
}

// bitsToBytes converts a slice of individual bits (MSB-first per byte)
// into a byte slice of the given length.
func bitsToBytes(bits []byte, length int) []byte {
	out := make([]byte, length)
	for i := 0; i < length*8 && i < len(bits); i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if bits[i] == 1 {
			out[byteIdx] |= 1 << bitIdx
		}
	}
	return out
}
