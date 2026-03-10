package main

import (
	"fmt"
	"strings"
)

// ObjectKind classifies a map object.
type ObjectKind int

const (
	KindUnclassified ObjectKind = iota
	KindPlayerStart
	KindWaypoint
	KindSupply
	KindTech
)

func (k ObjectKind) String() string {
	switch k {
	case KindPlayerStart:
		return "player_start"
	case KindWaypoint:
		return "waypoint"
	case KindSupply:
		return "supply"
	case KindTech:
		return "tech"
	default:
		return "unclassified"
	}
}

// MapObject represents a parsed object from the map file.
type MapObject struct {
	Name  string
	X, Y, Z float32
	Angle float32
	Flags int32
	Dict  []DictEntry
	Kind  ObjectKind

	// For waypoints
	WaypointID   int32
	WaypointName string
	PlayerNumber int // 1-based, 0 if not a player start
}

// ParseObject reads a single Object chunk payload.
func ParseObject(data []byte, version uint16, tbl SymbolTable) (*MapObject, error) {
	r := NewBinReader(data)

	x, err := r.ReadFloat32()
	if err != nil {
		return nil, fmt.Errorf("object x: %w", err)
	}
	y, err := r.ReadFloat32()
	if err != nil {
		return nil, fmt.Errorf("object y: %w", err)
	}
	z, err := r.ReadFloat32()
	if err != nil {
		return nil, fmt.Errorf("object z: %w", err)
	}
	if version <= 2 {
		z = 0
	}

	angle, err := r.ReadFloat32()
	if err != nil {
		return nil, fmt.Errorf("object angle: %w", err)
	}
	flags, err := r.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("object flags: %w", err)
	}
	name, err := r.ReadAsciiString()
	if err != nil {
		return nil, fmt.Errorf("object name: %w", err)
	}

	var dict []DictEntry
	if version >= 2 {
		dict, err = r.ReadDict(tbl)
		if err != nil {
			return nil, fmt.Errorf("object dict: %w", err)
		}
	}

	obj := &MapObject{
		Name:  name,
		X:     x,
		Y:     y,
		Z:     z,
		Angle: angle,
		Flags: flags,
		Dict:  dict,
	}

	classifyObject(obj)
	return obj, nil
}

func classifyObject(obj *MapObject) {
	// Check if it's a waypoint (has waypointID in dict)
	for _, e := range obj.Dict {
		if e.Key == "waypointID" && e.Type == DictInt {
			obj.WaypointID = e.IntVal
			// Find waypoint name
			for _, e2 := range obj.Dict {
				if e2.Key == "waypointName" && (e2.Type == DictAscii || e2.Type == DictUnicode) {
					obj.WaypointName = e2.StringVal
				}
			}
			// Check if it's a player start (Player_N_Start, 1-based)
			if strings.HasPrefix(obj.WaypointName, "Player_") && strings.HasSuffix(obj.WaypointName, "_Start") {
				mid := obj.WaypointName[7 : len(obj.WaypointName)-6]
				var n int
				if _, err := fmt.Sscanf(mid, "%d", &n); err == nil && n >= 1 {
					obj.Kind = KindPlayerStart
					obj.PlayerNumber = n
					return
				}
			}
			obj.Kind = KindWaypoint
			return
		}
	}

	if supplyNames[obj.Name] {
		obj.Kind = KindSupply
		return
	}
	if techNames[obj.Name] {
		obj.Kind = KindTech
		return
	}
}
