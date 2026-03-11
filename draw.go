package main

import (
	"image"
	"image/color"
	"strings"
)

// scaleImage does nearest-neighbor upscale so the larger dimension equals targetSize.
func scaleImage(img image.Image, targetSize int) *image.NRGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	var dstW, dstH int
	if srcW >= srcH {
		dstW = targetSize
		dstH = srcH * targetSize / srcW
	} else {
		dstH = targetSize
		dstW = srcW * targetSize / srcH
	}
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		srcY := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			srcX := x * srcW / dstW
			r, g, b, a := img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
			})
		}
	}
	return dst
}

// worldToPixel maps world coordinates to pixel coordinates.
// World origin is top-left; Y increases downward in world space.
func worldToPixel(objX, objY, extentX, extentY float64, imgW, imgH int) (int, int) {
	px := int(objX / extentX * float64(imgW))
	py := int(objY / extentY * float64(imgH))
	return px, py
}

func drawFilledCircle(img *image.NRGBA, cx, cy, radius int, c color.NRGBA) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				x, y := cx+dx, cy+dy
				if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
}

func drawRing(img *image.NRGBA, cx, cy, radius, thickness int, c color.NRGBA) {
	outer2 := radius * radius
	inner := radius - thickness
	inner2 := inner * inner
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			d2 := dx*dx + dy*dy
			if d2 <= outer2 && d2 >= inner2 {
				x, y := cx+dx, cy+dy
				if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
}

const fontScale = 3 // 5x7 glyphs drawn at 3x = 15x21 pixels

func drawBitmapString(img *image.NRGBA, x, y int, s string, c color.NRGBA) {
	s = strings.ToUpper(s)
	curX := x
	for _, ch := range s {
		glyph, ok := bitmapGlyphs[ch]
		if !ok {
			curX += 6 * fontScale // advance even for unknown chars
			continue
		}
		for row := 0; row < 7; row++ {
			bits := glyph[row]
			for col := 0; col < 5; col++ {
				if bits&(0x80>>uint(col)) != 0 {
					// Draw a fontScale x fontScale block
					for sy := 0; sy < fontScale; sy++ {
						for sx := 0; sx < fontScale; sx++ {
							px := curX + col*fontScale + sx
							py := y + row*fontScale + sy
							if px >= 0 && px < img.Bounds().Dx() && py >= 0 && py < img.Bounds().Dy() {
								img.SetNRGBA(px, py, c)
							}
						}
					}
				}
			}
		}
		curX += 6 * fontScale // 5 pixels + 1 spacing, scaled
	}
}

// drawBitmapStringCentered draws text centered horizontally on cx.
func drawBitmapStringCentered(img *image.NRGBA, cx, y int, s string, c color.NRGBA) {
	w := len([]rune(strings.ToUpper(s))) * 6 * fontScale
	drawBitmapString(img, cx-w/2, y, s, c)
}

var (
	colorBlue   = color.NRGBA{R: 60, G: 120, B: 255, A: 255}
	colorRed    = color.NRGBA{R: 255, G: 60, B: 60, A: 255}
	colorBlack  = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	colorGold   = color.NRGBA{R: 255, G: 200, B: 0, A: 255}
	colorPurple = color.NRGBA{R: 180, G: 60, B: 255, A: 255}
	colorWhite  = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

type playerAssignment struct {
	Name       string
	Team       int // 1 or 2
	StartIndex int // index into sorted player starts
	Faction    SubFaction
}

// annotateMap draws player starts, supply, and tech markers onto the image.
func annotateMap(img *image.NRGBA, md *MapData, assignments []playerAssignment, supply []*MapObject, tech []*MapObject) {
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()

	markerRadius := imgW / 80
	if markerRadius < 8 {
		markerRadius = 8
	}
	outlineThickness := markerRadius / 4
	if outlineThickness < 2 {
		outlineThickness = 2
	}

	// Draw supply markers
	for _, s := range supply {
		px, py := worldToPixel(float64(s.X), float64(s.Y), md.ExtentX, md.ExtentY, imgW, imgH)
		drawBitmapStringCentered(img, px, py-7*fontScale/2, "$", colorGold)
	}

	// Draw tech markers
	for _, t := range tech {
		px, py := worldToPixel(float64(t.X), float64(t.Y), md.ExtentX, md.ExtentY, imgW, imgH)
		drawBitmapStringCentered(img, px, py-7*fontScale/2, "T", colorPurple)
	}

	// Collect and sort player starts
	var starts []*MapObject
	for _, obj := range md.Objects {
		if obj.Kind == KindPlayerStart {
			starts = append(starts, obj)
		}
	}
	sortStarts(starts)

	// Draw player start assignments
	for _, a := range assignments {
		if a.StartIndex < 0 || a.StartIndex >= len(starts) {
			continue
		}
		start := starts[a.StartIndex]
		px, py := worldToPixel(float64(start.X), float64(start.Y), md.ExtentX, md.ExtentY, imgW, imgH)

		teamColor := colorBlue
		if a.Team == 2 {
			teamColor = colorRed
		}

		// Black outline ring then filled team color circle
		drawFilledCircle(img, px, py, markerRadius+outlineThickness, colorBlack)
		drawFilledCircle(img, px, py, markerRadius, teamColor)

		// Player name above
		nameY := py - markerRadius - outlineThickness - 7*fontScale - 4
		drawBitmapStringCentered(img, px, nameY, a.Name, colorWhite)
		// Drop shadow for legibility
		drawBitmapStringCentered(img, px+1, nameY+1, a.Name, colorBlack)
		drawBitmapStringCentered(img, px, nameY, a.Name, colorWhite)

		// Faction label below
		factionY := py + markerRadius + outlineThickness + 4
		label := string(a.Faction.Major) + "/" + string(a.Faction.Sub)
		drawBitmapStringCentered(img, px+1, factionY+1, label, colorBlack)
		drawBitmapStringCentered(img, px, factionY, label, colorWhite)
	}
}

// sortStarts sorts player starts by PlayerNumber.
func sortStarts(starts []*MapObject) {
	for i := 1; i < len(starts); i++ {
		for j := i; j > 0 && starts[j].PlayerNumber < starts[j-1].PlayerNumber; j-- {
			starts[j], starts[j-1] = starts[j-1], starts[j]
		}
	}
}
