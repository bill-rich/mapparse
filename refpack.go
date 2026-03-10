package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Decompress detects the EAR\0 header and decompresses RefPack data.
// Returns the raw data unchanged if no compression header is found.
func Decompress(data []byte) ([]byte, error) {
	if len(data) >= 8 && data[0] == 'E' && data[1] == 'A' && data[2] == 'R' && data[3] == 0 {
		uncompSize := binary.LittleEndian.Uint32(data[4:8])
		return refpackDecode(data[8:], int(uncompSize))
	}
	return data, nil
}

// refpackDecode implements the EA RefPack (LZ77 variant) decompressor.
// Ported from Core/Libraries/Source/Compression/EAC/refdecode.cpp
func refpackDecode(src []byte, expectedSize int) ([]byte, error) {
	if len(src) < 2 {
		return nil, errors.New("refpack: data too short")
	}

	s := 0 // source index

	// Read 2-byte big-endian type
	packType := (int(src[s]) << 8) | int(src[s+1])
	s += 2

	// Determine size field width (3 or 4 bytes)
	ssize := 3
	if packType&0x8000 != 0 {
		ssize = 4
	}

	// If bit 0x100 is set, skip the compressed size field
	if packType&0x100 != 0 {
		s += ssize
	}

	// Read uncompressed size (big-endian, ssize bytes)
	if s+ssize > len(src) {
		return nil, errors.New("refpack: truncated header")
	}
	ulen := 0
	for i := 0; i < ssize; i++ {
		ulen = (ulen << 8) | int(src[s])
		s++
	}

	if expectedSize > 0 && ulen != expectedSize {
		return nil, fmt.Errorf("refpack: size mismatch: header says %d, expected %d", ulen, expectedSize)
	}

	dst := make([]byte, 0, ulen)

	for {
		if s >= len(src) {
			return nil, errors.New("refpack: unexpected end of stream")
		}
		first := src[s]
		s++

		if first&0x80 == 0 {
			// Short form: 2-byte command
			if s >= len(src) {
				return nil, errors.New("refpack: truncated short command")
			}
			second := src[s]
			s++

			// Copy literal bytes
			run := int(first & 3)
			if s+run > len(src) {
				return nil, errors.New("refpack: truncated literal in short command")
			}
			dst = append(dst, src[s:s+run]...)
			s += run

			// Copy from back-reference
			offset := (int(first&0x60) << 3) + int(second) + 1
			copyLen := int((first&0x1c)>>2) + 3
			if offset > len(dst) {
				return nil, fmt.Errorf("refpack: back-reference offset %d exceeds output size %d", offset, len(dst))
			}
			refPos := len(dst) - offset
			for i := 0; i < copyLen; i++ {
				dst = append(dst, dst[refPos+i])
			}

		} else if first&0x40 == 0 {
			// Int form: 3-byte command
			if s+2 > len(src) {
				return nil, errors.New("refpack: truncated int command")
			}
			second := src[s]
			third := src[s+1]
			s += 2

			// Copy literal bytes
			run := int(second >> 6)
			if s+run > len(src) {
				return nil, errors.New("refpack: truncated literal in int command")
			}
			dst = append(dst, src[s:s+run]...)
			s += run

			// Copy from back-reference
			offset := (int(second&0x3f) << 8) + int(third) + 1
			copyLen := int(first&0x3f) + 4
			if offset > len(dst) {
				return nil, fmt.Errorf("refpack: back-reference offset %d exceeds output size %d", offset, len(dst))
			}
			refPos := len(dst) - offset
			for i := 0; i < copyLen; i++ {
				dst = append(dst, dst[refPos+i])
			}

		} else if first&0x20 == 0 {
			// Very-int form: 4-byte command
			if s+3 > len(src) {
				return nil, errors.New("refpack: truncated very-int command")
			}
			second := src[s]
			third := src[s+1]
			forth := src[s+2]
			s += 3

			// Copy literal bytes
			run := int(first & 3)
			if s+run > len(src) {
				return nil, errors.New("refpack: truncated literal in very-int command")
			}
			dst = append(dst, src[s:s+run]...)
			s += run

			// Copy from back-reference
			offset := (int(first&0x10)>>4)<<16 + (int(second) << 8) + int(third) + 1
			copyLen := (int(first&0x0c)>>2)<<8 + int(forth) + 5
			if offset > len(dst) {
				return nil, fmt.Errorf("refpack: back-reference offset %d exceeds output size %d", offset, len(dst))
			}
			refPos := len(dst) - offset
			for i := 0; i < copyLen; i++ {
				dst = append(dst, dst[refPos+i])
			}

		} else {
			// Literal or EOF
			run := (int(first&0x1f) << 2) + 4
			if run <= 112 {
				// Literal copy
				if s+run > len(src) {
					return nil, errors.New("refpack: truncated literal block")
				}
				dst = append(dst, src[s:s+run]...)
				s += run
			} else {
				// EOF with 0..3 trailing literal bytes
				run = int(first & 3)
				if s+run > len(src) {
					return nil, errors.New("refpack: truncated EOF literal")
				}
				dst = append(dst, src[s:s+run]...)
				s += run
				break
			}
		}
	}

	return dst, nil
}
