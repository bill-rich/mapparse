package main

import (
	"fmt"
	"math"
	"strings"
)

// Ground-path distances between player start positions.
//
// This mirrors the engine's map-cache computation (Core MapUtil.cpp,
// computeStartSpotPathDistances): a cell of the heightmap grid is impassable
// when the height spread across its four corners exceeds the pathfind cliff
// limit (WorldHeightMap::setCellCliffFlagFromHeights) or when its center lies
// under a water polygon; cells outside the playable border are always
// impassable; bridge spans stamp a passable corridor back in. An 8-way flood
// fill (cost 10 straight / 14 diagonal, no corner cutting) then gives the
// ground distance between every pair of start positions.

// Constants mirrored from the engine.
const (
	mapHeightScale  = mapXYFactor / 16.0 // MAP_HEIGHT_SCALE: world height units per raw heightmap step
	cliffSlopeLimit = 9.8                // PATHFIND_CLIFF_SLOPE_LIMIT_F (WorldHeightMap.cpp)
	unreachableBase = 1000000.0          // added to straight-line distance for disconnected pairs

	// Landmark bridges are single objects whose true span comes from INI
	// geometry data this parser doesn't have; use a generous fixed half-span
	// along the object's facing angle instead.
	landmarkBridgeHalfSpan = 80.0

	flagBridgePoint1 = 0x10 // FLAG_BRIDGE_POINT1 (Common/MapObject.h)
	flagBridgePoint2 = 0x20 // FLAG_BRIDGE_POINT2
)

// IPoint3 is an integer world-space point (polygon trigger vertex).
type IPoint3 struct {
	X, Y, Z int32
}

// WaterArea is a water polygon trigger in world coordinates. The water
// surface height is the z of the first vertex (TerrainLogic::getWaterHandle
// uses getPoint(0)->z).
type WaterArea struct {
	Points                 []IPoint3
	WaterZ                 float64
	minX, minY, maxX, maxY int32
}

func newWaterArea(points []IPoint3) WaterArea {
	a := WaterArea{Points: points, WaterZ: float64(points[0].Z)}
	a.minX, a.maxX = points[0].X, points[0].X
	a.minY, a.maxY = points[0].Y, points[0].Y
	for _, p := range points[1:] {
		if p.X < a.minX {
			a.minX = p.X
		}
		if p.X > a.maxX {
			a.maxX = p.X
		}
		if p.Y < a.minY {
			a.minY = p.Y
		}
		if p.Y > a.maxY {
			a.maxY = p.Y
		}
	}
	return a
}

// contains reports whether the world point (px, py) lies inside the polygon.
// Same crossing test as PolygonTrigger::pointInTrigger.
func (a *WaterArea) contains(px, py int32) bool {
	if px < a.minX || py < a.minY || px > a.maxX || py > a.maxY {
		return false
	}
	inside := false
	n := len(a.Points)
	for i := 0; i < n; i++ {
		pt1 := a.Points[i]
		pt2 := a.Points[(i+1)%n]
		if pt1.Y == pt2.Y {
			continue // ignore horizontal lines
		}
		if pt1.Y < py && pt2.Y < py {
			continue
		}
		if pt1.Y >= py && pt2.Y >= py {
			continue
		}
		if pt1.X < px && pt2.X < px {
			continue
		}
		dy := pt2.Y - pt1.Y
		dx := pt2.X - pt1.X
		intersectionX := float64(pt1.X) + float64(dx)*float64(py-pt1.Y)/float64(dy)
		if intersectionX >= float64(px) {
			inside = !inside
		}
	}
	return inside
}

// parsePolygonTriggers reads the PolygonTriggers chunk, keeping water areas.
// Field layout follows PolygonTrigger::ParsePolygonTriggersDataChunk.
func parsePolygonTriggers(chunk *Chunk, md *MapData) error {
	r := NewBinReader(chunk.Data)
	count, err := r.ReadInt32()
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}
	for ; count > 0; count-- {
		if _, err := r.ReadAsciiString(); err != nil { // trigger name
			return fmt.Errorf("trigger name: %w", err)
		}
		if chunk.Version >= 4 {
			if _, err := r.ReadAsciiString(); err != nil { // layer name
				return fmt.Errorf("layer name: %w", err)
			}
		}
		if _, err := r.ReadInt32(); err != nil { // trigger id
			return fmt.Errorf("trigger id: %w", err)
		}
		isWater := false
		if chunk.Version >= 2 {
			b, err := r.ReadByte()
			if err != nil {
				return fmt.Errorf("isWater: %w", err)
			}
			isWater = b != 0
		}
		if chunk.Version >= 3 {
			if _, err := r.ReadByte(); err != nil { // isRiver
				return fmt.Errorf("isRiver: %w", err)
			}
			if _, err := r.ReadInt32(); err != nil { // riverStart
				return fmt.Errorf("riverStart: %w", err)
			}
		}
		numPoints, err := r.ReadInt32()
		if err != nil {
			return fmt.Errorf("numPoints: %w", err)
		}
		var points []IPoint3
		for i := int32(0); i < numPoints; i++ {
			var p IPoint3
			if p.X, err = r.ReadInt32(); err != nil {
				return fmt.Errorf("point x: %w", err)
			}
			if p.Y, err = r.ReadInt32(); err != nil {
				return fmt.Errorf("point y: %w", err)
			}
			if p.Z, err = r.ReadInt32(); err != nil {
				return fmt.Errorf("point z: %w", err)
			}
			if isWater {
				points = append(points, p)
			}
		}
		if isWater && len(points) >= 3 {
			md.WaterAreas = append(md.WaterAreas, newWaterArea(points))
		}
	}
	if chunk.Version == 1 {
		// Maps from before water areas existed imply a default global water
		// plane at the old water position z=7 (the engine builds it from
		// TheGlobalData water extents; whole-map coverage is equivalent here).
		md.LegacyDefaultWater = true
	}
	return nil
}

// BridgeSpan is a walkable corridor over otherwise impassable terrain.
type BridgeSpan struct {
	Name         string
	FromX, FromY float32
	ToX, ToY     float32
	Landmark     bool // single-object bridge; span length is approximated
}

// deriveBridges extracts bridge spans from the object list. Sectional river
// bridges are two consecutive objects flagged BRIDGE_POINT1 then
// BRIDGE_POINT2 (same adjacency rule as W3DBridgeBuffer::loadBridges);
// landmark bridges are a single object (name contains "bridge") whose span
// runs along its facing angle.
func deriveBridges(md *MapData) {
	var pending *MapObject
	for _, obj := range md.Objects {
		switch {
		case obj.Flags&flagBridgePoint1 != 0:
			pending = obj
		case obj.Flags&flagBridgePoint2 != 0:
			if pending != nil {
				md.Bridges = append(md.Bridges, BridgeSpan{
					Name:  pending.Name,
					FromX: pending.X, FromY: pending.Y,
					ToX: obj.X, ToY: obj.Y,
				})
				pending = nil
			}
		default:
			pending = nil
			if obj.Kind == KindUnclassified && strings.Contains(strings.ToLower(obj.Name), "bridge") {
				dx := float32(math.Cos(float64(obj.Angle)) * landmarkBridgeHalfSpan)
				dy := float32(math.Sin(float64(obj.Angle)) * landmarkBridgeHalfSpan)
				md.Bridges = append(md.Bridges, BridgeSpan{
					Name:  obj.Name,
					FromX: obj.X - dx, FromY: obj.Y - dy,
					ToX: obj.X + dx, ToY: obj.Y + dy,
					Landmark: true,
				})
			}
		}
	}
}

// PassabilityGrid is the per-cell ground passability of a map. Cells sit
// between heightmap grid points, so the grid is (Width-1) x (Height-1).
type PassabilityGrid struct {
	CellsX, CellsY int
	Passable       []bool // index y*CellsX + x
	borderSize     int
}

// BuildPassabilityGrid computes ground passability from heights, water and
// bridges. Returns nil when the map has no height data.
func BuildPassabilityGrid(md *MapData) *PassabilityGrid {
	if len(md.Heights) == 0 || md.Width < 2 || md.Height < 2 {
		return nil
	}
	w, h := int(md.Width), int(md.Height)
	border := int(md.BorderSize)
	g := &PassabilityGrid{CellsX: w - 1, CellsY: h - 1, borderSize: border}
	g.Passable = make([]bool, g.CellsX*g.CellsY)

	heightAt := func(x, y int) float64 {
		return float64(md.Heights[y*w+x]) * mapHeightScale
	}

	for y := 0; y < g.CellsY; y++ {
		for x := 0; x < g.CellsX; x++ {
			// Units cannot detour through the decorative border.
			if x < border || x > w-2-border || y < border || y > h-2-border {
				continue
			}
			h1 := heightAt(x, y)
			h2 := heightAt(x+1, y)
			h3 := heightAt(x, y+1)
			h4 := heightAt(x+1, y+1)
			minZ := math.Min(math.Min(h1, h2), math.Min(h3, h4))
			maxZ := math.Max(math.Max(h1, h2), math.Max(h3, h4))
			if maxZ-minZ > cliffSlopeLimit {
				continue
			}
			terrainZ := (h1 + h2 + h3 + h4) * 0.25
			wx := (float64(x-border) + 0.5) * mapXYFactor
			wy := (float64(y-border) + 0.5) * mapXYFactor
			underwater := false
			if md.LegacyDefaultWater && terrainZ < 7.0 {
				underwater = true
			}
			if !underwater {
				for i := range md.WaterAreas {
					a := &md.WaterAreas[i]
					if terrainZ < a.WaterZ && a.contains(int32(math.Floor(wx)), int32(math.Floor(wy))) {
						underwater = true
						break
					}
				}
			}
			if underwater {
				continue
			}
			g.Passable[y*g.CellsX+x] = true
		}
	}

	// Bridges reconnect the grid across the water/chasm they span.
	for _, br := range md.Bridges {
		fx, fy := float64(br.FromX), float64(br.FromY)
		tx, ty := float64(br.ToX), float64(br.ToY)
		length := math.Hypot(tx-fx, ty-fy)
		steps := int(length/(mapXYFactor*0.5)) + 1
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)
			cx := int(math.Floor((fx+(tx-fx)*t)/mapXYFactor)) + border
			cy := int(math.Floor((fy+(ty-fy)*t)/mapXYFactor)) + border
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := cx+dx, cy+dy
					if nx >= 0 && nx < g.CellsX && ny >= 0 && ny < g.CellsY {
						g.Passable[ny*g.CellsX+nx] = true
					}
				}
			}
		}
	}

	return g
}

// cellFor maps a world position to a grid cell, nudged to the nearest
// passable cell within a small radius (start spots can sit against a cliff
// edge that the coarse cell test flags impassable).
func (g *PassabilityGrid) cellFor(wx, wy float64) int {
	cx := int(math.Floor(wx/mapXYFactor)) + g.borderSize
	cy := int(math.Floor(wy/mapXYFactor)) + g.borderSize
	if cx < 0 {
		cx = 0
	}
	if cx > g.CellsX-1 {
		cx = g.CellsX - 1
	}
	if cy < 0 {
		cy = 0
	}
	if cy > g.CellsY-1 {
		cy = g.CellsY - 1
	}
	if !g.Passable[cy*g.CellsX+cx] {
		for radius := 1; radius <= 5; radius++ {
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					nx, ny := cx+dx, cy+dy
					if nx >= 0 && nx < g.CellsX && ny >= 0 && ny < g.CellsY && g.Passable[ny*g.CellsX+nx] {
						return ny*g.CellsX + nx
					}
				}
			}
		}
	}
	return cy*g.CellsX + cx
}

// floodFill runs Dial's algorithm (bucket ring; all in-flight costs lie
// within 14 of the current cost, so residues mod 15 never collide) from the
// given cell. Costs are 10 straight / 14 diagonal, with no corner cutting
// between two blocked cells. Returns per-cell cost, -1 for unreachable.
func (g *PassabilityGrid) floodFill(start int) []int {
	dist := make([]int, len(g.Passable))
	for i := range dist {
		dist[i] = -1
	}
	var buckets [15][]int
	dist[start] = 0
	buckets[0] = append(buckets[0], start)
	pending := 1
	curCost := 0
	for pending > 0 {
		b := &buckets[curCost%15]
		if len(*b) == 0 {
			curCost++
			continue
		}
		cell := (*b)[len(*b)-1]
		*b = (*b)[:len(*b)-1]
		pending--
		if dist[cell] != curCost {
			continue // stale entry, a shorter path got there first
		}
		cx := cell % g.CellsX
		cy := cell / g.CellsX
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx, ny := cx+dx, cy+dy
				if nx < 0 || nx >= g.CellsX || ny < 0 || ny >= g.CellsY {
					continue
				}
				ncell := ny*g.CellsX + nx
				if !g.Passable[ncell] {
					continue
				}
				if dx != 0 && dy != 0 {
					if !g.Passable[cy*g.CellsX+nx] || !g.Passable[ny*g.CellsX+cx] {
						continue
					}
				}
				ncost := curCost + 10
				if dx != 0 && dy != 0 {
					ncost = curCost + 14
				}
				if dist[ncell] < 0 || ncost < dist[ncell] {
					dist[ncell] = ncost
					buckets[ncost%15] = append(buckets[ncost%15], ncell)
					pending++
				}
			}
		}
	}
	return dist
}

// PathDistPair is the ground-path distance between two player starts.
type PathDistPair struct {
	FromPlayer, ToPlayer int // 1-based player numbers
	PathDistance         float64
	StraightDistance     float64
	Connected            bool
}

// ComputePathDistances returns the pairwise ground distances between all
// player start positions, ordered by (from, to) player number. Returns nil
// when the map has no height data or fewer than two starts.
func ComputePathDistances(md *MapData) []PathDistPair {
	var starts []*MapObject
	for _, obj := range md.Objects {
		if obj.Kind == KindPlayerStart {
			starts = append(starts, obj)
		}
	}
	if len(starts) < 2 {
		return nil
	}
	sortByPlayerNumber(starts)

	grid := BuildPassabilityGrid(md)
	if grid == nil {
		return nil
	}

	var pairs []PathDistPair
	for i := 0; i < len(starts)-1; i++ {
		dist := grid.floodFill(grid.cellFor(float64(starts[i].X), float64(starts[i].Y)))
		for j := i + 1; j < len(starts); j++ {
			straight := math.Hypot(float64(starts[i].X-starts[j].X), float64(starts[i].Y-starts[j].Y))
			cost := dist[grid.cellFor(float64(starts[j].X), float64(starts[j].Y))]
			p := PathDistPair{
				FromPlayer:       starts[i].PlayerNumber,
				ToPlayer:         starts[j].PlayerNumber,
				StraightDistance: straight,
				Connected:        cost >= 0,
			}
			if cost >= 0 {
				p.PathDistance = float64(cost) * (mapXYFactor / 10.0)
				// The grid metric can undershoot slightly on pure diagonals.
				if p.PathDistance < straight {
					p.PathDistance = straight
				}
			} else {
				// Disconnected (island vs island): very far, but keep the
				// straight-line ordering between such pairs.
				p.PathDistance = unreachableBase + straight
			}
			pairs = append(pairs, p)
		}
	}
	return pairs
}

func sortByPlayerNumber(starts []*MapObject) {
	for i := 1; i < len(starts); i++ {
		for j := i; j > 0 && starts[j-1].PlayerNumber > starts[j].PlayerNumber; j-- {
			starts[j-1], starts[j] = starts[j], starts[j-1]
		}
	}
}

// runPassability prints an ASCII rendering of the passability grid with
// player starts and bridges marked, for eyeballing against the real map.
func runPassability(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mapparse passability <mapfile>")
	}
	md, err := ParseMap(args[0])
	if err != nil {
		return err
	}
	grid := BuildPassabilityGrid(md)
	if grid == nil {
		return fmt.Errorf("map has no height data")
	}

	marks := map[int]byte{}
	for _, obj := range md.Objects {
		if obj.Kind == KindPlayerStart && obj.PlayerNumber >= 1 && obj.PlayerNumber <= 9 {
			marks[grid.cellFor(float64(obj.X), float64(obj.Y))] = byte('0' + obj.PlayerNumber)
		}
	}

	// Downsample to a terminal-friendly width; a sample cell is impassable
	// when the majority of covered cells are.
	stride := 1
	for grid.CellsX/stride > 120 {
		stride++
	}
	for y := grid.CellsY - 1; y >= 0; y -= stride { // world y grows upward
		line := make([]byte, 0, grid.CellsX/stride+1)
		for x := 0; x < grid.CellsX; x += stride {
			ch := byte('#')
			blocked := 0
			total := 0
			var mark byte
			for sy := y; sy > y-stride && sy >= 0; sy-- {
				for sx := x; sx < x+stride && sx < grid.CellsX; sx++ {
					total++
					if !grid.Passable[sy*grid.CellsX+sx] {
						blocked++
					}
					if m, ok := marks[sy*grid.CellsX+sx]; ok {
						mark = m
					}
				}
			}
			if blocked*2 < total {
				ch = '.'
			}
			if mark != 0 {
				ch = mark
			}
			line = append(line, ch)
		}
		fmt.Println(string(line))
	}

	fmt.Println()
	for _, br := range md.Bridges {
		kind := "sectional"
		if br.Landmark {
			kind = "landmark (span approximated)"
		}
		fmt.Printf("bridge %s: (%.0f, %.0f) -> (%.0f, %.0f) [%s]\n", br.Name, br.FromX, br.FromY, br.ToX, br.ToY, kind)
	}
	for _, p := range ComputePathDistances(md) {
		note := ""
		if !p.Connected {
			note = "  DISCONNECTED"
		}
		fmt.Printf("player %d <-> %d: path=%.0f straight=%.0f (x%.2f)%s\n",
			p.FromPlayer, p.ToPlayer, p.PathDistance, p.StraightDistance, p.PathDistance/p.StraightDistance, note)
	}
	return nil
}
