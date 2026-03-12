package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// SymbolTable maps chunk IDs to their string names.
type SymbolTable map[uint32]string

// ReadTOC parses the DataChunk table of contents starting at data[0].
// Returns the symbol table and the byte offset where chunk data begins.
func ReadTOC(data []byte) (SymbolTable, int, error) {
	if len(data) < 8 {
		return nil, 0, errors.New("datachunk: data too short for TOC")
	}
	// Check magic: CkMp
	if data[0] != 'C' || data[1] != 'k' || data[2] != 'M' || data[3] != 'p' {
		return nil, 0, fmt.Errorf("datachunk: bad magic %q, expected CkMp", string(data[:4]))
	}

	pos := 4
	count := int(int32(binary.LittleEndian.Uint32(data[pos:])))
	pos += 4

	tbl := make(SymbolTable, count)
	for i := 0; i < count; i++ {
		if pos >= len(data) {
			return nil, 0, errors.New("datachunk: truncated TOC")
		}
		nameLen := int(data[pos])
		pos++
		if pos+nameLen+4 > len(data) {
			return nil, 0, errors.New("datachunk: truncated TOC entry")
		}
		name := string(data[pos : pos+nameLen])
		pos += nameLen
		id := binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		tbl[id] = name
	}

	return tbl, pos, nil
}

// ChunkReader reads sequential chunks from a DataChunk byte slice.
type ChunkReader struct {
	data []byte
	pos  int
	end  int
	tbl  SymbolTable
}

// NewChunkReader creates a reader over the given data range.
func NewChunkReader(data []byte, start, end int, tbl SymbolTable) *ChunkReader {
	return &ChunkReader{data: data, pos: start, end: end, tbl: tbl}
}

// Chunk represents a single data chunk.
type Chunk struct {
	Name    string
	Version uint16
	Data    []byte // raw payload bytes
}

// Next reads the next chunk. Returns nil, nil at end of data.
func (r *ChunkReader) Next() (*Chunk, error) {
	// Chunk header: uint32 id + uint16 version + int32 dataSize = 10 bytes
	if r.pos+10 > r.end {
		if r.pos == r.end {
			return nil, nil // clean EOF
		}
		// Less than a full header remaining — treat as end
		return nil, nil
	}

	id := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	version := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	dataSize := int(int32(binary.LittleEndian.Uint32(r.data[r.pos:])))
	r.pos += 4

	if dataSize < 0 || r.pos+dataSize > r.end {
		return nil, fmt.Errorf("datachunk: chunk data size %d exceeds available data", dataSize)
	}

	name := r.tbl[id]
	payload := r.data[r.pos : r.pos+dataSize]
	r.pos += dataSize

	return &Chunk{Name: name, Version: version, Data: payload}, nil
}

// SubReader creates a ChunkReader over a chunk's payload for reading nested chunks.
func SubReader(c *Chunk, tbl SymbolTable) *ChunkReader {
	return NewChunkReader(c.Data, 0, len(c.Data), tbl)
}

// Primitive readers for chunk payloads.

// BinReader reads typed values from a byte slice at a tracked position.
type BinReader struct {
	data []byte
	pos  int
}

func NewBinReader(data []byte) *BinReader {
	return &BinReader{data: data}
}

func (r *BinReader) Remaining() int { return len(r.data) - r.pos }

func (r *BinReader) ReadInt16() (int16, error) {
	if r.pos+2 > len(r.data) {
		return 0, errors.New("binreader: read past end")
	}
	v := int16(binary.LittleEndian.Uint16(r.data[r.pos:]))
	r.pos += 2
	return v, nil
}

func (r *BinReader) Skip(n int) error {
	if r.pos+n > len(r.data) {
		return fmt.Errorf("binreader: skip %d bytes past end", n)
	}
	r.pos += n
	return nil
}

func (r *BinReader) ReadByte() (byte, error) {
	if r.pos+1 > len(r.data) {
		return 0, errors.New("binreader: read past end")
	}
	v := r.data[r.pos]
	r.pos++
	return v, nil
}

func (r *BinReader) ReadInt32() (int32, error) {
	if r.pos+4 > len(r.data) {
		return 0, errors.New("binreader: read past end")
	}
	v := int32(binary.LittleEndian.Uint32(r.data[r.pos:]))
	r.pos += 4
	return v, nil
}

func (r *BinReader) ReadUint16() (uint16, error) {
	if r.pos+2 > len(r.data) {
		return 0, errors.New("binreader: read past end")
	}
	v := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return v, nil
}

func (r *BinReader) ReadFloat32() (float32, error) {
	if r.pos+4 > len(r.data) {
		return 0, errors.New("binreader: read past end")
	}
	bits := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return math.Float32frombits(bits), nil
}

func (r *BinReader) ReadAsciiString() (string, error) {
	length, err := r.ReadUint16()
	if err != nil {
		return "", err
	}
	n := int(length)
	if r.pos+n > len(r.data) {
		return "", errors.New("binreader: string read past end")
	}
	s := string(r.data[r.pos : r.pos+n])
	r.pos += n
	return s, nil
}

func (r *BinReader) ReadUnicodeString() (string, error) {
	charCount, err := r.ReadUint16()
	if err != nil {
		return "", err
	}
	n := int(charCount) * 2
	if r.pos+n > len(r.data) {
		return "", errors.New("binreader: unicode string read past end")
	}
	// Decode UTF-16LE to string
	runes := make([]rune, charCount)
	for i := 0; i < int(charCount); i++ {
		runes[i] = rune(binary.LittleEndian.Uint16(r.data[r.pos+i*2:]))
	}
	r.pos += n
	return string(runes), nil
}

// Dict types matching the C++ Dict::DataType enum.
const (
	DictBool    = 0
	DictInt     = 1
	DictReal    = 2
	DictAscii   = 3
	DictUnicode = 4
)

// DictEntry represents one key-value pair in a Dict.
type DictEntry struct {
	Key       string
	Type      int
	BoolVal   bool
	IntVal    int32
	RealVal   float32
	StringVal string
}

// ReadDict reads a Dict from the binary reader, using the symbol table to resolve key names.
func (r *BinReader) ReadDict(tbl SymbolTable) ([]DictEntry, error) {
	pairCount, err := r.ReadUint16()
	if err != nil {
		return nil, fmt.Errorf("dict: %w", err)
	}

	entries := make([]DictEntry, 0, pairCount)
	for i := 0; i < int(pairCount); i++ {
		keyAndType, err := r.ReadInt32()
		if err != nil {
			return nil, fmt.Errorf("dict entry %d: %w", i, err)
		}

		dtype := int(keyAndType & 0xff)
		keyID := uint32(keyAndType >> 8)
		keyName := tbl[keyID]

		entry := DictEntry{Key: keyName, Type: dtype}

		switch dtype {
		case DictBool:
			b, err := r.ReadByte()
			if err != nil {
				return nil, fmt.Errorf("dict bool: %w", err)
			}
			entry.BoolVal = b != 0
		case DictInt:
			v, err := r.ReadInt32()
			if err != nil {
				return nil, fmt.Errorf("dict int: %w", err)
			}
			entry.IntVal = v
		case DictReal:
			v, err := r.ReadFloat32()
			if err != nil {
				return nil, fmt.Errorf("dict real: %w", err)
			}
			entry.RealVal = v
		case DictAscii:
			v, err := r.ReadAsciiString()
			if err != nil {
				return nil, fmt.Errorf("dict ascii: %w", err)
			}
			entry.StringVal = v
		case DictUnicode:
			v, err := r.ReadUnicodeString()
			if err != nil {
				return nil, fmt.Errorf("dict unicode: %w", err)
			}
			entry.StringVal = v
		default:
			return nil, fmt.Errorf("dict: unknown type %d", dtype)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
