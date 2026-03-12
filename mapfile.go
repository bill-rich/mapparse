package main

import (
	"fmt"
	"os"
)

const mapXYFactor = 10.0

// MapData holds the parsed map information.
type MapData struct {
	Width      int32
	Height     int32
	BorderSize int32
	ExtentX    float64 // world-space width
	ExtentY    float64 // world-space height
	Objects    []*MapObject
	Heights    []byte          // raw height values per grid cell
	BlendTile  *BlendTileData  // parsed blend tile data (nil if chunk absent)
}

// ParseMap reads and parses a C&C Generals .map file.
func ParseMap(filename string) (*MapData, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Decompress if RefPack-compressed
	data, err := Decompress(raw)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}

	// Parse table of contents
	tbl, dataStart, err := ReadTOC(data)
	if err != nil {
		return nil, fmt.Errorf("read TOC: %w", err)
	}

	// Walk top-level chunks
	reader := NewChunkReader(data, dataStart, len(data), tbl)
	md := &MapData{}

	for {
		chunk, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("read chunk: %w", err)
		}
		if chunk == nil {
			break
		}

		switch chunk.Name {
		case "HeightMapData":
			if err := parseHeightMap(chunk, md); err != nil {
				return nil, fmt.Errorf("HeightMapData: %w", err)
			}
		case "BlendTileData":
			bt, err := parseBlendTileData(chunk)
			if err != nil {
				return nil, fmt.Errorf("BlendTileData: %w", err)
			}
			md.BlendTile = bt
		case "ObjectsList":
			if err := parseObjectsList(chunk, tbl, md); err != nil {
				return nil, fmt.Errorf("ObjectsList: %w", err)
			}
		}
	}

	// Compute map extent
	mapDX := md.Width - 2*md.BorderSize
	mapDY := md.Height - 2*md.BorderSize
	md.ExtentX = float64(mapDX) * mapXYFactor
	md.ExtentY = float64(mapDY) * mapXYFactor

	return md, nil
}

func parseHeightMap(chunk *Chunk, md *MapData) error {
	r := NewBinReader(chunk.Data)

	width, err := r.ReadInt32()
	if err != nil {
		return fmt.Errorf("width: %w", err)
	}
	height, err := r.ReadInt32()
	if err != nil {
		return fmt.Errorf("height: %w", err)
	}

	md.Width = width
	md.Height = height

	if chunk.Version >= 3 {
		bs, err := r.ReadInt32()
		if err != nil {
			return fmt.Errorf("borderSize: %w", err)
		}
		md.BorderSize = bs
	}

	if chunk.Version >= 4 {
		numBorders, err := r.ReadInt32()
		if err != nil {
			return fmt.Errorf("numBorders: %w", err)
		}
		// Skip boundary data: numBorders * 2 * int32
		if err := r.Skip(int(numBorders) * 8); err != nil {
			return fmt.Errorf("boundary data: %w", err)
		}
	}

	// Read height data
	dataSize, err := r.ReadInt32()
	if err != nil {
		return fmt.Errorf("height dataSize: %w", err)
	}
	if int(dataSize) > r.Remaining() {
		return fmt.Errorf("height data truncated: want %d, have %d", dataSize, r.Remaining())
	}
	md.Heights = make([]byte, dataSize)
	copy(md.Heights, r.data[r.pos:r.pos+int(dataSize)])
	r.pos += int(dataSize)

	return nil
}

func parseObjectsList(chunk *Chunk, tbl SymbolTable, md *MapData) error {
	sub := SubReader(chunk, tbl)

	for {
		objChunk, err := sub.Next()
		if err != nil {
			return fmt.Errorf("read object chunk: %w", err)
		}
		if objChunk == nil {
			break
		}
		if objChunk.Name != "Object" {
			continue
		}

		obj, err := ParseObject(objChunk.Data, objChunk.Version, tbl)
		if err != nil {
			return fmt.Errorf("parse object: %w", err)
		}
		md.Objects = append(md.Objects, obj)
	}

	return nil
}
