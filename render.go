package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

func runRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	mapPath := fs.String("map", "", "Path to .map file (required)")
	outPath := fs.String("out", "", "Output PNG path (default: <mapname>_terrain.png)")
	scale := fs.Int("scale", 1, "Pixels per grid cell")
	noShading := fs.Bool("no-shading", false, "Disable height shading")
	texturesDir := fs.String("textures", "", "Path to Terrain directory with TGA files and Terrain.ini")
	fs.Parse(args)

	if *mapPath == "" {
		return fmt.Errorf("render: -map is required")
	}
	if *scale < 1 {
		return fmt.Errorf("render: -scale must be >= 1")
	}

	// Default output path
	if *outPath == "" {
		base := strings.TrimSuffix(filepath.Base(*mapPath), filepath.Ext(*mapPath))
		*outPath = filepath.Join(filepath.Dir(*mapPath), base+"_terrain.png")
	}

	md, err := ParseMap(*mapPath)
	if err != nil {
		return fmt.Errorf("parse map: %w", err)
	}

	if md.BlendTile == nil {
		return fmt.Errorf("map has no BlendTileData chunk")
	}
	if len(md.Heights) == 0 {
		return fmt.Errorf("map has no height data")
	}

	// Store grid dimensions on BlendTile for cellTerrainName lookups
	md.BlendTile.NumCellsX = md.Width
	md.BlendTile.NumCellsY = md.Height

	// Load texture atlas cache if textures dir provided
	var texCache *textureCache
	if *texturesDir != "" {
		iniPath := filepath.Join(*texturesDir, "Terrain.ini")
		ini, err := parseTerrainINI(iniPath)
		if err != nil {
			return fmt.Errorf("parse Terrain.ini: %w", err)
		}
		texCache = newTextureCache(*texturesDir, ini)
	}

	playW := int(md.Width - 2*md.BorderSize)
	playH := int(md.Height - 2*md.BorderSize)
	if playW <= 0 || playH <= 0 {
		return fmt.Errorf("invalid playable area: %d x %d", playW, playH)
	}

	imgW := playW * *scale
	imgH := playH * *scale

	// Compute min/max height across the playable area
	border := int(md.BorderSize)
	gridW := int(md.Width)
	var minH, maxH byte
	minH = 255
	for py := 0; py < playH; py++ {
		for px := 0; px < playW; px++ {
			idx := (py+border)*gridW + (px + border)
			if idx < len(md.Heights) {
				h := md.Heights[idx]
				if h < minH {
					minH = h
				}
				if h > maxH {
					maxH = h
				}
			}
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, imgW, imgH))

	for py := 0; py < playH; py++ {
		for px := 0; px < playW; px++ {
			cellX := px + border
			cellY := py + border

			// Height shading factor
			heightIdx := cellY*gridW + cellX
			var shade float64 = 1.0
			if !*noShading && maxH != minH {
				var h byte
				if heightIdx < len(md.Heights) {
					h = md.Heights[heightIdx]
				}
				t := float64(h-minH) / float64(maxH-minH)
				shade = 0.7 + 0.6*t
			}

			// Try texture atlas sampling
			var textured bool
			if texCache != nil {
				textured = renderCellFromAtlas(img, md.BlendTile, texCache, cellX, cellY, px, py, *scale, shade)
			}

			if !textured {
				// Fallback to color palette
				name := md.BlendTile.cellTerrainName(cellX, cellY)
				base := terrainColor(name)
				c := applyShade(base, shade)
				fillBlock(img, px**scale, py**scale, *scale, c)
			}
		}
	}

	f, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}

	fmt.Printf("Rendered %d x %d terrain image to %s\n", imgW, imgH, *outPath)
	return nil
}

// renderCellFromAtlas tries to render a cell using texture atlas data.
// Returns false if the atlas is not available for this cell's texture class.
func renderCellFromAtlas(img *image.NRGBA, bt *BlendTileData, cache *textureCache,
	cellX, cellY, px, py, scale int, shade float64) bool {

	ndx := cellY*int(bt.NumCellsX) + cellX
	if ndx < 0 || ndx >= len(bt.TileIndices) {
		return false
	}
	rawIndex := bt.TileIndices[ndx]
	tileNum := int(rawIndex) >> 2

	// Find the texture class for this tile
	var tc *TextureClass
	for i := range bt.TextureClasses {
		c := &bt.TextureClasses[i]
		if tileNum >= int(c.FirstTile) && tileNum < int(c.FirstTile+c.NumTiles) {
			tc = c
			break
		}
	}
	if tc == nil {
		return false
	}

	atlas := cache.get(tc.Name, tc.Width)
	if atlas == nil {
		return false
	}

	pixels := atlas.sampleCell(rawIndex, tc.FirstTile, tc.Width, scale)
	if pixels == nil {
		return false
	}
	outX := px * scale
	outY := py * scale
	for sy := 0; sy < scale; sy++ {
		for sx := 0; sx < scale; sx++ {
			p := pixels[sy*scale+sx]
			c := applyShadeNRGBA(p, shade)
			img.SetNRGBA(outX+sx, outY+sy, c)
		}
	}
	return true
}

func applyShade(base color.RGBA, shade float64) color.NRGBA {
	return color.NRGBA{
		R: clampByte(float64(base.R) * shade),
		G: clampByte(float64(base.G) * shade),
		B: clampByte(float64(base.B) * shade),
		A: 255,
	}
}

func applyShadeNRGBA(p color.NRGBA, shade float64) color.NRGBA {
	return color.NRGBA{
		R: clampByte(float64(p.R) * shade),
		G: clampByte(float64(p.G) * shade),
		B: clampByte(float64(p.B) * shade),
		A: 255,
	}
}

func fillBlock(img *image.NRGBA, x, y, size int, c color.NRGBA) {
	for sy := 0; sy < size; sy++ {
		for sx := 0; sx < size; sx++ {
			img.SetNRGBA(x+sx, y+sy, c)
		}
	}
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
