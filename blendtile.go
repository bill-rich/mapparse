package main

import "fmt"

type TextureClass struct {
	Name      string
	FirstTile int32
	NumTiles  int32
	Width     int32
}

type BlendTileData struct {
	NumCellsX      int32
	NumCellsY      int32
	TileIndices    []int16
	TextureClasses []TextureClass
}

func parseBlendTileData(chunk *Chunk) (*BlendTileData, error) {
	r := NewBinReader(chunk.Data)

	dataSize, err := r.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("dataSize: %w", err)
	}

	bt := &BlendTileData{}

	// Read tile indices (int16 array, dataSize entries)
	bt.TileIndices = make([]int16, dataSize)
	for i := int32(0); i < dataSize; i++ {
		v, err := r.ReadInt16()
		if err != nil {
			return nil, fmt.Errorf("tile index %d: %w", i, err)
		}
		bt.TileIndices[i] = v
	}

	// Skip blend tile indices (dataSize * 2 bytes)
	if err := r.Skip(int(dataSize) * 2); err != nil {
		return nil, fmt.Errorf("skip blend indices: %w", err)
	}

	// Version >= 6: extra blend indices
	if chunk.Version >= 6 {
		if err := r.Skip(int(dataSize) * 2); err != nil {
			return nil, fmt.Errorf("skip extra blend indices: %w", err)
		}
	}

	// Version >= 5: cliff info indices
	if chunk.Version >= 5 {
		if err := r.Skip(int(dataSize) * 2); err != nil {
			return nil, fmt.Errorf("skip cliff indices: %w", err)
		}
	}

	// Version >= 7: cell cliff state bit flags
	// Each cell uses 1 bit, packed into bytes. Total = ceil(dataSize / 8).
	if chunk.Version >= 7 {
		flagBytes := (int(dataSize) + 7) / 8
		if err := r.Skip(flagBytes); err != nil {
			return nil, fmt.Errorf("skip cliff flags: %w", err)
		}
	}

	// numBitmapTiles
	if _, err := r.ReadInt32(); err != nil {
		return nil, fmt.Errorf("numBitmapTiles: %w", err)
	}

	// numBlendedTiles
	if _, err := r.ReadInt32(); err != nil {
		return nil, fmt.Errorf("numBlendedTiles: %w", err)
	}

	// Version >= 5: numCliffInfo
	if chunk.Version >= 5 {
		if _, err := r.ReadInt32(); err != nil {
			return nil, fmt.Errorf("numCliffInfo: %w", err)
		}
	}

	// numTextureClasses
	numTC, err := r.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("numTextureClasses: %w", err)
	}

	bt.TextureClasses = make([]TextureClass, numTC)
	for i := int32(0); i < numTC; i++ {
		firstTile, err := r.ReadInt32()
		if err != nil {
			return nil, fmt.Errorf("textureClass %d firstTile: %w", i, err)
		}
		numTiles, err := r.ReadInt32()
		if err != nil {
			return nil, fmt.Errorf("textureClass %d numTiles: %w", i, err)
		}
		width, err := r.ReadInt32()
		if err != nil {
			return nil, fmt.Errorf("textureClass %d width: %w", i, err)
		}
		// legacy field (discard)
		if _, err := r.ReadInt32(); err != nil {
			return nil, fmt.Errorf("textureClass %d legacy: %w", i, err)
		}
		name, err := r.ReadAsciiString()
		if err != nil {
			return nil, fmt.Errorf("textureClass %d name: %w", i, err)
		}
		bt.TextureClasses[i] = TextureClass{
			Name:      name,
			FirstTile: firstTile,
			NumTiles:  numTiles,
			Width:     width,
		}
	}

	// Remaining data (edge texture classes, blended tile entries, cliff info) is skipped.
	return bt, nil
}

// cellTerrainName returns the terrain type name for the cell at grid coordinates (x, y).
// width must be set on BlendTileData before calling.
func (bt *BlendTileData) cellTerrainName(x, y int) string {
	ndx := y*int(bt.NumCellsX) + x
	if ndx < 0 || ndx >= len(bt.TileIndices) {
		return ""
	}
	tileNdx := int(bt.TileIndices[ndx]) >> 2
	for _, tc := range bt.TextureClasses {
		if tileNdx >= int(tc.FirstTile) && tileNdx < int(tc.FirstTile+tc.NumTiles) {
			return tc.Name
		}
	}
	return ""
}
