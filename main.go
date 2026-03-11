package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "assign" {
		if err := runAssign(os.Args[2:]); err != nil {
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
	Extent       extent            `json:"extent"`
	PlayerStarts []jsonPosition    `json:"player_starts"`
	Supply       []jsonPosition    `json:"supply"`
	Tech         []jsonPosition    `json:"tech"`
	Waypoints    []jsonWaypoint    `json:"waypoints,omitempty"`
	Other        []jsonObject      `json:"other,omitempty"`
}

type extent struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type jsonPosition struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
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
		case KindWaypoint:
			waypoints = append(waypoints, obj)
		default:
			other = append(other, obj)
		}
	}

	sort.Slice(starts, func(i, j int) bool { return starts[i].PlayerNumber < starts[j].PlayerNumber })

	if asJSON {
		out := jsonOutput{
			Extent: extent{Width: md.ExtentX, Height: md.ExtentY},
		}
		for _, s := range starts {
			out.PlayerStarts = append(out.PlayerStarts, jsonPosition{X: s.X, Y: s.Y})
		}
		for _, s := range supply {
			out.Supply = append(out.Supply, jsonPosition{X: s.X, Y: s.Y})
		}
		for _, s := range tech {
			out.Tech = append(out.Tech, jsonPosition{X: s.X, Y: s.Y})
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
		fmt.Printf("  %s: (%.1f, %.1f)\n", s.Name, s.X, s.Y)
	}
	fmt.Println()

	fmt.Printf("Tech positions (%d):\n", len(tech))
	for _, t := range tech {
		fmt.Printf("  %s: (%.1f, %.1f)\n", t.Name, t.X, t.Y)
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
