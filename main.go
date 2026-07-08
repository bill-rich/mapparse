package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "worldinfo" {
		if err := runWorldInfo(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "passability" {
		if err := runPassability(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	dump := flag.Bool("dump", false, "Dump ALL objects (for discovering template names)")
	jsonOut := flag.Bool("json", false, "Output as JSON")
	verbose := flag.Bool("verbose", false, "Include unclassified objects")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: mapparse [flags] <mapfile>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	md, err := ParseMap(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *dump {
		outputDump(md, *jsonOut)
	} else {
		outputSummary(md, *jsonOut, *verbose)
	}
}

// JSON output types

type jsonOutput struct {
	Extent        extent         `json:"extent"`
	PlayerStarts  []jsonPosition `json:"player_starts"`
	Supply        []jsonPosition `json:"supply"`
	Tech          []jsonPosition `json:"tech"`
	Garrison      []jsonPosition `json:"garrison"`
	Bridges       []jsonBridge   `json:"bridges,omitempty"`
	PathDistances []jsonPathDist `json:"path_distances,omitempty"`
	Waypoints     []jsonWaypoint `json:"waypoints,omitempty"`
	Other         []jsonObject   `json:"other,omitempty"`
}

type jsonBridge struct {
	Name     string  `json:"name"`
	FromX    float32 `json:"from_x"`
	FromY    float32 `json:"from_y"`
	ToX      float32 `json:"to_x"`
	ToY      float32 `json:"to_y"`
	Landmark bool    `json:"landmark,omitempty"`
}

type jsonPathDist struct {
	FromPlayer       int     `json:"from_player"`
	ToPlayer         int     `json:"to_player"`
	PathDistance     float64 `json:"path_distance"`
	StraightDistance float64 `json:"straight_distance"`
	Connected        bool    `json:"connected"`
}

type extent struct {
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	GridWidth  int32   `json:"grid_width"`
	GridHeight int32   `json:"grid_height"`
	BorderSize int32   `json:"border_size"`
}

type jsonPosition struct {
	Name         string  `json:"name,omitempty"`
	PlayerNumber int     `json:"player_number,omitempty"`
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
	// Amount is the available supply cash (supply objects only). Overridden is
	// true when a start-of-game script set the value instead of the INI default.
	Amount     int  `json:"amount,omitempty"`
	Overridden bool `json:"overridden,omitempty"`
}

type jsonWaypoint struct {
	Name string  `json:"name"`
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
}

type jsonObject struct {
	Name  string       `json:"name"`
	X     float32      `json:"x"`
	Y     float32      `json:"y"`
	Z     float32      `json:"z"`
	Angle float32      `json:"angle"`
	Flags int32        `json:"flags"`
	Dict  []jsonDictEntry `json:"dict,omitempty"`
}

type jsonDictEntry struct {
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

func outputSummary(md *MapData, asJSON bool, verbose bool) {
	var starts []*MapObject
	var supply []*MapObject
	var tech []*MapObject
	var garrison []*MapObject
	var waypoints []*MapObject
	var other []*MapObject

	for _, obj := range md.Objects {
		switch obj.Kind {
		case KindPlayerStart:
			starts = append(starts, obj)
		case KindSupply:
			supply = append(supply, obj)
		case KindTech:
			tech = append(tech, obj)
		case KindGarrison:
			garrison = append(garrison, obj)
		case KindWaypoint:
			waypoints = append(waypoints, obj)
		default:
			other = append(other, obj)
		}
	}

	sort.Slice(starts, func(i, j int) bool { return starts[i].PlayerNumber < starts[j].PlayerNumber })

	if asJSON {
		out := jsonOutput{
			Extent: extent{
				Width:      md.ExtentX,
				Height:     md.ExtentY,
				GridWidth:  md.Width,
				GridHeight: md.Height,
				BorderSize: md.BorderSize,
			},
		}
		for _, s := range starts {
			out.PlayerStarts = append(out.PlayerStarts, jsonPosition{PlayerNumber: s.PlayerNumber, X: s.X, Y: s.Y})
		}
		for _, s := range supply {
			amount, overridden, _ := resolveSupplyAmount(md, s)
			out.Supply = append(out.Supply, jsonPosition{Name: s.Name, X: s.X, Y: s.Y, Amount: amount, Overridden: overridden})
		}
		for _, s := range tech {
			out.Tech = append(out.Tech, jsonPosition{Name: s.Name, X: s.X, Y: s.Y})
		}
		for _, g := range garrison {
			out.Garrison = append(out.Garrison, jsonPosition{Name: g.Name, X: g.X, Y: g.Y})
		}
		for _, b := range md.Bridges {
			out.Bridges = append(out.Bridges, jsonBridge{Name: b.Name, FromX: b.FromX, FromY: b.FromY, ToX: b.ToX, ToY: b.ToY, Landmark: b.Landmark})
		}
		for _, p := range ComputePathDistances(md) {
			out.PathDistances = append(out.PathDistances, jsonPathDist{
				FromPlayer:       p.FromPlayer,
				ToPlayer:         p.ToPlayer,
				PathDistance:     p.PathDistance,
				StraightDistance: p.StraightDistance,
				Connected:        p.Connected,
			})
		}
		for _, w := range waypoints {
			out.Waypoints = append(out.Waypoints, jsonWaypoint{Name: w.WaypointName, X: w.X, Y: w.Y})
		}
		if verbose {
			for _, o := range other {
				out.Other = append(out.Other, toJSONObject(o))
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out)
		return
	}

	// Human-readable output
	fmt.Printf("Map extent: %.0f x %.0f (grid: %d x %d, border: %d)\n",
		md.ExtentX, md.ExtentY, md.Width, md.Height, md.BorderSize)
	fmt.Println()

	fmt.Printf("Player starts (%d):\n", len(starts))
	for _, s := range starts {
		fmt.Printf("  Player %d: (%.1f, %.1f)\n", s.PlayerNumber, s.X, s.Y)
	}
	fmt.Println()

	fmt.Printf("Supply positions (%d):\n", len(supply))
	for _, s := range supply {
		amount, overridden, known := resolveSupplyAmount(md, s)
		switch {
		case overridden:
			fmt.Printf("  %s: (%.1f, %.1f) amount=%d (script override)\n", s.Name, s.X, s.Y, amount)
		case known:
			fmt.Printf("  %s: (%.1f, %.1f) amount=%d\n", s.Name, s.X, s.Y, amount)
		default:
			fmt.Printf("  %s: (%.1f, %.1f) amount=unknown\n", s.Name, s.X, s.Y)
		}
	}
	fmt.Println()

	fmt.Printf("Tech positions (%d):\n", len(tech))
	for _, t := range tech {
		fmt.Printf("  %s: (%.1f, %.1f)\n", t.Name, t.X, t.Y)
	}
	fmt.Println()

	fmt.Printf("Garrisonable buildings (%d):\n", len(garrison))
	for _, g := range garrison {
		fmt.Printf("  %s: (%.1f, %.1f)\n", g.Name, g.X, g.Y)
	}

	if len(md.Bridges) > 0 {
		fmt.Println()
		fmt.Printf("Bridges (%d):\n", len(md.Bridges))
		for _, b := range md.Bridges {
			kind := "sectional"
			if b.Landmark {
				kind = "landmark, span approximated"
			}
			fmt.Printf("  %s: (%.1f, %.1f) -> (%.1f, %.1f) [%s]\n", b.Name, b.FromX, b.FromY, b.ToX, b.ToY, kind)
		}
	}

	if pairs := ComputePathDistances(md); len(pairs) > 0 {
		fmt.Println()
		fmt.Printf("Ground-path distances between starts:\n")
		for _, p := range pairs {
			note := ""
			if !p.Connected {
				note = "  DISCONNECTED"
			}
			fmt.Printf("  player %d <-> %d: path=%.0f straight=%.0f (x%.2f)%s\n",
				p.FromPlayer, p.ToPlayer, p.PathDistance, p.StraightDistance, p.PathDistance/p.StraightDistance, note)
		}
	}

	if len(waypoints) > 0 {
		fmt.Println()
		fmt.Printf("Other waypoints (%d):\n", len(waypoints))
		for _, w := range waypoints {
			fmt.Printf("  %s: (%.1f, %.1f)\n", w.WaypointName, w.X, w.Y)
		}
	}

	if verbose && len(other) > 0 {
		fmt.Println()
		fmt.Printf("Unclassified objects (%d):\n", len(other))
		for _, o := range other {
			fmt.Printf("  %s: (%.1f, %.1f, %.1f) angle=%.1f flags=%d\n",
				o.Name, o.X, o.Y, o.Z, o.Angle, o.Flags)
		}
	}
}

// resolveSupplyAmount returns the available cash for a supply object. A
// start-of-game script override (keyed by editor name) wins over the INI
// default (StartingBoxes * valuePerSupplyBox). known is false when neither a
// script value nor a known template default applies.
func resolveSupplyAmount(md *MapData, obj *MapObject) (amount int, overridden bool, known bool) {
	if obj.EditorName != "" {
		if cash, ok := md.SupplyOverrides[obj.EditorName]; ok {
			return cash, true, true
		}
	}
	if def, ok := defaultSupplyAmount(obj.Name); ok {
		return def, false, true
	}
	return 0, false, false
}

func outputDump(md *MapData, asJSON bool) {
	if asJSON {
		var out []jsonObject
		for _, obj := range md.Objects {
			out = append(out, toJSONObject(obj))
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(out)
		return
	}

	fmt.Printf("Map extent: %.0f x %.0f (grid: %d x %d, border: %d)\n",
		md.ExtentX, md.ExtentY, md.Width, md.Height, md.BorderSize)
	fmt.Printf("Total objects: %d\n\n", len(md.Objects))

	for i, obj := range md.Objects {
		fmt.Printf("[%d] %s (%s)\n", i, obj.Name, obj.Kind)
		fmt.Printf("    pos=(%.1f, %.1f, %.1f) angle=%.1f flags=%d\n",
			obj.X, obj.Y, obj.Z, obj.Angle, obj.Flags)
		if obj.Kind == KindPlayerStart {
			fmt.Printf("    Player %d start\n", obj.PlayerNumber)
		}
		if obj.Kind == KindWaypoint || obj.Kind == KindPlayerStart {
			fmt.Printf("    waypointName=%q waypointID=%d\n", obj.WaypointName, obj.WaypointID)
		}
		if len(obj.Dict) > 0 {
			fmt.Printf("    dict:\n")
			for _, e := range obj.Dict {
				fmt.Printf("      %s = %s\n", e.Key, dictValueStr(e))
			}
		}
	}
}

func toJSONObject(obj *MapObject) jsonObject {
	jo := jsonObject{
		Name:  obj.Name,
		X:     obj.X,
		Y:     obj.Y,
		Z:     obj.Z,
		Angle: obj.Angle,
		Flags: obj.Flags,
	}
	for _, e := range obj.Dict {
		jo.Dict = append(jo.Dict, jsonDictEntry{
			Key:   e.Key,
			Type:  dictTypeName(e.Type),
			Value: dictValue(e),
		})
	}
	return jo
}

func dictTypeName(t int) string {
	switch t {
	case DictBool:
		return "bool"
	case DictInt:
		return "int"
	case DictReal:
		return "real"
	case DictAscii:
		return "ascii"
	case DictUnicode:
		return "unicode"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func dictValue(e DictEntry) interface{} {
	switch e.Type {
	case DictBool:
		return e.BoolVal
	case DictInt:
		return e.IntVal
	case DictReal:
		return e.RealVal
	case DictAscii, DictUnicode:
		return e.StringVal
	default:
		return nil
	}
}

func dictValueStr(e DictEntry) string {
	switch e.Type {
	case DictBool:
		return fmt.Sprintf("%v", e.BoolVal)
	case DictInt:
		return fmt.Sprintf("%d", e.IntVal)
	case DictReal:
		return fmt.Sprintf("%g", e.RealVal)
	case DictAscii, DictUnicode:
		return fmt.Sprintf("%q", e.StringVal)
	default:
		return "?"
	}
}

// runWorldInfo dumps the WorldInfo chunk's dict. Use Go's %q verb so any
// non-printable bytes in the stored values (e.g. truncated nameLookupTag
// strings that pick up garbage from the next field) show up explicitly.
func runWorldInfo(args []string) error {
	fs := flag.NewFlagSet("worldinfo", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output as JSON")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: mapparse worldinfo [--json] <mapfile>")
		os.Exit(1)
	}
	md, err := ParseMap(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		var entries []jsonDictEntry
		for _, e := range md.WorldDict {
			entries = append(entries, jsonDictEntry{
				Key:   e.Key,
				Type:  dictTypeName(e.Type),
				Value: dictValue(e),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	for _, e := range md.WorldDict {
		fmt.Printf("%s = %s\n", e.Key, dictValueStr(e))
	}
	return nil
}
