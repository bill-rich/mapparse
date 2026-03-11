package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
)

// decodeTGA reads an uncompressed (type 2) or RLE (type 10) truecolor TGA.
func decodeTGA(filename string) (image.Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	if len(data) < 18 {
		return nil, fmt.Errorf("tga: file too short")
	}

	idLen := int(data[0])
	imgType := data[2]
	width := int(binary.LittleEndian.Uint16(data[12:14]))
	height := int(binary.LittleEndian.Uint16(data[14:16]))
	bpp := int(data[16])
	descriptor := data[17]

	if imgType != 2 && imgType != 10 {
		return nil, fmt.Errorf("tga: unsupported image type %d (need 2 or 10)", imgType)
	}
	if bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("tga: unsupported bpp %d (need 24 or 32)", bpp)
	}

	pixelBytes := bpp / 8
	pixelData := data[18+idLen:]

	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	// Descriptor bit 5: 0 = bottom-up (default TGA), 1 = top-down
	topDown := descriptor&0x20 != 0

	if imgType == 2 {
		// Uncompressed
		expected := width * height * pixelBytes
		if len(pixelData) < expected {
			return nil, fmt.Errorf("tga: pixel data too short (have %d, need %d)", len(pixelData), expected)
		}
		off := 0
		for y := 0; y < height; y++ {
			destY := y
			if !topDown {
				destY = height - 1 - y
			}
			for x := 0; x < width; x++ {
				b, g, r := pixelData[off], pixelData[off+1], pixelData[off+2]
				a := uint8(255)
				if pixelBytes == 4 {
					a = pixelData[off+3]
				}
				img.SetNRGBA(x, destY, color.NRGBA{R: r, G: g, B: b, A: a})
				off += pixelBytes
			}
		}
	} else {
		// RLE (type 10)
		off := 0
		px := 0
		total := width * height
		for px < total {
			if off >= len(pixelData) {
				return nil, fmt.Errorf("tga: unexpected end of RLE data")
			}
			header := pixelData[off]
			off++
			count := int(header&0x7F) + 1

			if header&0x80 != 0 {
				// Run-length packet
				if off+pixelBytes > len(pixelData) {
					return nil, fmt.Errorf("tga: unexpected end of RLE run data")
				}
				b, g, r := pixelData[off], pixelData[off+1], pixelData[off+2]
				a := uint8(255)
				if pixelBytes == 4 {
					a = pixelData[off+3]
				}
				off += pixelBytes
				c := color.NRGBA{R: r, G: g, B: b, A: a}
				for i := 0; i < count && px < total; i++ {
					x := px % width
					y := px / width
					destY := y
					if !topDown {
						destY = height - 1 - y
					}
					img.SetNRGBA(x, destY, c)
					px++
				}
			} else {
				// Raw packet
				for i := 0; i < count && px < total; i++ {
					if off+pixelBytes > len(pixelData) {
						return nil, fmt.Errorf("tga: unexpected end of RLE raw data")
					}
					b, g, r := pixelData[off], pixelData[off+1], pixelData[off+2]
					a := uint8(255)
					if pixelBytes == 4 {
						a = pixelData[off+3]
					}
					off += pixelBytes
					x := px % width
					y := px / width
					destY := y
					if !topDown {
						destY = height - 1 - y
					}
					img.SetNRGBA(x, destY, color.NRGBA{R: r, G: g, B: b, A: a})
					px++
				}
			}
		}
	}

	return img, nil
}
