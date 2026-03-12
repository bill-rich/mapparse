package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
)

// TerrainINI maps texture class names to TGA filenames.
type TerrainINI map[string]string

// parseTerrainINI reads a Terrain.ini file and returns the class name → TGA filename map.
// When a class name appears multiple times, the first occurrence wins.
func parseTerrainINI(path string) (TerrainINI, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ini := make(TerrainINI)
	scanner := bufio.NewScanner(f)
	var currentName string

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		// Strip comments
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "Terrain ") {
			currentName = strings.TrimSpace(line[len("Terrain "):])
		} else if strings.HasPrefix(line, "Texture") && currentName != "" {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				tga := strings.TrimSpace(parts[1])
				if _, exists := ini[currentName]; !exists {
					ini[currentName] = tga
				}
			}
		} else if strings.EqualFold(line, "End") {
			currentName = ""
		}
	}

	return ini, scanner.Err()
}

// textureAtlas holds a loaded TGA tile atlas for a texture class.
type textureAtlas struct {
	img       image.Image
	tileWidth int // tile size in pixels (atlas pixel width / class tile grid width)
}

// textureCache loads and caches texture atlases.
type textureCache struct {
	dir     string
	ini     TerrainINI
	atlases map[string]*textureAtlas // keyed by texture class name
	missing map[string]bool          // classes we already tried and failed to load
}

func newTextureCache(dir string, ini TerrainINI) *textureCache {
	return &textureCache{
		dir:     dir,
		ini:     ini,
		atlases: make(map[string]*textureAtlas),
		missing: make(map[string]bool),
	}
}

// get returns the atlas for a texture class, or nil if unavailable.
func (tc *textureCache) get(className string, classWidth int32) *textureAtlas {
	if a, ok := tc.atlases[className]; ok {
		return a
	}
	if tc.missing[className] {
		return nil
	}

	tgaName, ok := tc.ini[className]
	if !ok {
		tc.missing[className] = true
		return nil
	}

	// Try case-insensitive file lookup
	path := tc.findFile(tgaName)
	if path == "" {
		tc.missing[className] = true
		return nil
	}

	img, err := decodeTGA(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load %s: %v\n", path, err)
		tc.missing[className] = true
		return nil
	}

	tileW := img.Bounds().Dx() / int(classWidth)
	a := &textureAtlas{img: img, tileWidth: tileW}
	tc.atlases[className] = a
	return a
}

// findFile does a case-insensitive search for filename in the texture directory.
func (tc *textureCache) findFile(name string) string {
	// Try exact match first
	path := filepath.Join(tc.dir, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Case-insensitive scan
	entries, err := os.ReadDir(tc.dir)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(name)
	for _, e := range entries {
		if strings.ToLower(e.Name()) == lower {
			return filepath.Join(tc.dir, e.Name())
		}
	}
	return ""
}

// sampleCell returns the color for a cell from the tile atlas.
// rawIndex is the raw tile index from BlendTileData, tc is the texture class.
func (a *textureAtlas) sampleCell(rawIndex int16, classFirstTile int32, classWidth int32, scale int) []color.NRGBA {
	tileNum := int(rawIndex) >> 2
	subCell := int(rawIndex) & 3

	localTile := tileNum - int(classFirstTile)
	tileCol := localTile % int(classWidth)
	tileRow := localTile / int(classWidth)

	tw := a.tileWidth
	halfTile := tw / 2

	// Sub-cell quadrant within the tile
	subX := (subCell & 1) * halfTile
	subY := (subCell >> 1) * halfTile

	// Top-left pixel of this cell's quadrant in the atlas
	atlasX := tileCol*tw + subX
	atlasY := tileRow*tw + subY

	// Sample scale×scale pixels from the halfTile×halfTile quadrant
	pixels := make([]color.NRGBA, scale*scale)
	for sy := 0; sy < scale; sy++ {
		for sx := 0; sx < scale; sx++ {
			// Map output pixel to source quadrant pixel
			srcX := atlasX + sx*halfTile/scale
			srcY := atlasY + sy*halfTile/scale
			r, g, b, aa := a.img.At(srcX, srcY).RGBA()
			pixels[sy*scale+sx] = color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(aa >> 8),
			}
		}
	}
	return pixels
}
