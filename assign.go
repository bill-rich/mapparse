package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runAssign(args []string) error {
	fs := flag.NewFlagSet("assign", flag.ExitOnError)
	mapDir := fs.String("map", "", "Path to map directory (required)")
	team1Str := fs.String("team1", "", "Comma-separated team 1 players (required)")
	team2Str := fs.String("team2", "", "Comma-separated team 2 players (required)")
	outPath := fs.String("out", "", "Output PNG path (default: <mapdir>/assigned.png)")
	targetSize := fs.Int("target", 2048, "Target size for larger image dimension")
	fs.Parse(args)

	if *mapDir == "" || *team1Str == "" || *team2Str == "" {
		return fmt.Errorf("usage: mapparse assign -map <dir> -team1 <a,b> -team2 <c,d> [-out path] [-target size]")
	}

	team1 := strings.Split(*team1Str, ",")
	team2 := strings.Split(*team2Str, ",")

	// Derive basename from directory path
	basename := filepath.Base(*mapDir)
	mapFile := filepath.Join(*mapDir, basename+".map")
	tgaFile := filepath.Join(*mapDir, basename+".tga")

	if *outPath == "" {
		*outPath = filepath.Join(*mapDir, "assigned.png")
	}

	// Parse the map
	md, err := ParseMap(mapFile)
	if err != nil {
		return fmt.Errorf("parse map: %w", err)
	}

	// Collect starts, supply, tech
	var starts []*MapObject
	var supply []*MapObject
	var tech []*MapObject
	for _, obj := range md.Objects {
		switch obj.Kind {
		case KindPlayerStart:
			starts = append(starts, obj)
		case KindSupply:
			supply = append(supply, obj)
		case KindTech:
			tech = append(tech, obj)
		}
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].PlayerNumber < starts[j].PlayerNumber })

	// Assign positions
	assignments, err := assignPositions(team1, team2, len(starts))
	if err != nil {
		return err
	}

	// Assign factions
	hPath := historyPath()
	history := loadHistory(hPath)
	factions, wiped := assignFactions(team1, team2, history)
	if wiped {
		fmt.Println("NOTE: Faction history was too constraining; wiped and re-assigned.")
	}

	// Attach factions to assignments
	for i := range assignments {
		assignments[i].Faction = factions[assignments[i].Name]
	}

	// Save new faction history
	var newRecords []historyRecord
	for _, a := range assignments {
		newRecords = append(newRecords, historyRecord{
			Player: a.Name,
			Major:  a.Faction.Major,
			Sub:    a.Faction.Sub,
		})
	}
	if err := appendHistory(hPath, newRecords); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save faction history: %v\n", err)
	}

	// Decode TGA and render
	tgaImg, err := decodeTGA(tgaFile)
	if err != nil {
		return fmt.Errorf("decode tga: %w", err)
	}

	scaled := scaleImage(tgaImg, *targetSize)
	annotateMap(scaled, md, assignments, supply, tech)

	// Write PNG
	f, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()
	if err := png.Encode(f, scaled); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}

	// Print summary
	fmt.Printf("Map: %s (%d starts, %d supply, %d tech)\n", basename, len(starts), len(supply), len(tech))
	fmt.Printf("Output: %s\n\n", *outPath)

	fmt.Println("Team 1 (blue):")
	for _, a := range assignments {
		if a.Team == 1 {
			fmt.Printf("  %-12s  pos %d  %s/%s\n", a.Name, starts[a.StartIndex].PlayerNumber, a.Faction.Major, a.Faction.Sub)
		}
	}

	fmt.Println("\nTeam 2 (red):")
	for _, a := range assignments {
		if a.Team == 2 {
			fmt.Printf("  %-12s  pos %d  %s/%s\n", a.Name, starts[a.StartIndex].PlayerNumber, a.Faction.Major, a.Faction.Sub)
		}
	}

	return nil
}
