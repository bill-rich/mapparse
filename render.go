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

	playW := int(md.Width - 2*md.BorderSize)
	playH := int(md.Height - 2*md.BorderSize)
	if playW <= 0 || playH <= 0 {
		return fmt.Errorf("invalid playable area: %d x %d", playW, playH)
	}

	imgW := playW * *scale
	imgH := playH * *scale

	// Compute min/max height across the playable area
	border := int(md.BorderSize)
	width := int(md.Width)
	var minH, maxH byte
	minH = 255
	for py := 0; py < playH; py++ {
		for px := 0; px < playW; px++ {
			idx := (py+border)*width + (px + border)
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
			name := md.BlendTile.cellTerrainName(px+border, py+border)
			base := terrainColor(name)

			var c color.RGBA
			if *noShading || maxH == minH {
				c = base
			} else {
				idx := (py+border)*width + (px + border)
				var h byte
				if idx < len(md.Heights) {
					h = md.Heights[idx]
				}
				t := float64(h-minH) / float64(maxH-minH)
				shade := 0.7 + 0.6*t
				c = color.RGBA{
					R: clampByte(float64(base.R) * shade),
					G: clampByte(float64(base.G) * shade),
					B: clampByte(float64(base.B) * shade),
					A: 255,
				}
			}

			// Fill scale x scale block
			for sy := 0; sy < *scale; sy++ {
				for sx := 0; sx < *scale; sx++ {
					img.SetNRGBA(px**scale+sx, py**scale+sy, color.NRGBA{c.R, c.G, c.B, c.A})
				}
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

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
