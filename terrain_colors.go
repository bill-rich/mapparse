package main

import (
	"image/color"
	"strings"
)

var terrainKeywords = []struct {
	keyword string
	color   color.RGBA
}{
	{"cobblestone", color.RGBA{145, 140, 130, 255}},
	{"residential", color.RGBA{155, 150, 135, 255}},
	{"concrete", color.RGBA{165, 165, 160, 255}},
	{"asphalt", color.RGBA{90, 90, 90, 255}},
	{"mountain", color.RGBA{120, 115, 105, 255}},
	{"transition", color.RGBA{145, 145, 90, 255}},
	{"europe", color.RGBA{95, 135, 65, 255}},
	{"swiss", color.RGBA{85, 130, 55, 255}},
	{"china", color.RGBA{150, 140, 100, 255}},
	{"snow", color.RGBA{220, 225, 230, 255}},
	{"grass", color.RGBA{100, 145, 60, 255}},
	{"field", color.RGBA{130, 160, 70, 255}},
	{"beach", color.RGBA{210, 195, 150, 255}},
	{"desert", color.RGBA{180, 155, 105, 255}},
	{"sand", color.RGBA{195, 175, 130, 255}},
	{"cliff", color.RGBA{110, 105, 95, 255}},
	{"rock", color.RGBA{130, 125, 115, 255}},
	{"dirt", color.RGBA{140, 110, 75, 255}},
	{"mud", color.RGBA{115, 95, 65, 255}},
	{"urban", color.RGBA{150, 145, 135, 255}},
	{"wood", color.RGBA{110, 90, 55, 255}},
	{"water", color.RGBA{60, 100, 170, 255}},
	{"blend", color.RGBA{155, 150, 130, 255}},
}

var defaultTerrainColor = color.RGBA{150, 140, 120, 255}

func terrainColor(name string) color.RGBA {
	lower := strings.ToLower(name)
	for _, kw := range terrainKeywords {
		if strings.Contains(lower, kw.keyword) {
			return kw.color
		}
	}
	return defaultTerrainColor
}
