package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

type MajorFaction string
type SubFactionName string

const (
	FactionUSA   MajorFaction = "USA"
	FactionChina MajorFaction = "China"
	FactionGLA   MajorFaction = "GLA"
)

type SubFaction struct {
	Major MajorFaction
	Sub   SubFactionName
}

var allSubFactions = []SubFaction{
	{FactionUSA, "Vanilla"},
	{FactionUSA, "Airforce"},
	{FactionUSA, "Laser"},
	{FactionUSA, "Super"},
	{FactionChina, "Vanilla"},
	{FactionChina, "Tank"},
	{FactionChina, "Infantry"},
	{FactionChina, "Nuke"},
	{FactionGLA, "Vanilla"},
	{FactionGLA, "Toxin"},
	{FactionGLA, "Stealth"},
	{FactionGLA, "Demo"},
}

type historyRecord struct {
	Player string
	Major  MajorFaction
	Sub    SubFactionName
}

func historyPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "faction_history.txt"
	}
	return filepath.Join(filepath.Dir(exe), "faction_history.txt")
}

func loadHistory(path string) []historyRecord {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var records []historyRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 {
			continue
		}
		records = append(records, historyRecord{
			Player: parts[0],
			Major:  MajorFaction(parts[1]),
			Sub:    SubFactionName(parts[2]),
		})
	}
	return records
}

func appendHistory(path string, records []historyRecord) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, r := range records {
		fmt.Fprintf(f, "%s,%s,%s\n", r.Player, r.Major, r.Sub)
	}
	return nil
}

// playerHistory returns the most recent major faction and the set of all sub-factions played.
func playerHistory(records []historyRecord, player string) (lastMajor MajorFaction, playedSubs map[SubFaction]bool) {
	playedSubs = make(map[SubFaction]bool)
	lower := strings.ToLower(player)
	for _, r := range records {
		if strings.ToLower(r.Player) == lower {
			lastMajor = r.Major
			playedSubs[SubFaction{r.Major, r.Sub}] = true
		}
	}
	return
}

// assignFactions picks a sub-faction for each player respecting constraints:
// 1. No two players on the same team get the identical sub-faction
// 2. Player can't get same major faction two times in a row
// 3. Player can't get a sub-faction they've already played (ever)
func assignFactions(team1, team2 []string, history []historyRecord) (map[string]SubFaction, bool) {
	result := make(map[string]SubFaction)
	wiped := false

	ok := tryAssignTeam(team1, history, result) && tryAssignTeam(team2, history, result)
	if !ok {
		// Wipe history and retry
		wiped = true
		result = make(map[string]SubFaction)
		tryAssignTeam(team1, nil, result)
		tryAssignTeam(team2, nil, result)
	}
	return result, wiped
}

func tryAssignTeam(team []string, history []historyRecord, result map[string]SubFaction) bool {
	const maxRetries = 10

	for retry := 0; retry < maxRetries; retry++ {
		candidates := make([]SubFaction, len(allSubFactions))
		copy(candidates, allSubFactions)
		rand.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})

		teamResult := make(map[string]SubFaction)
		usedSubs := make(map[SubFaction]bool)
		success := true

		for _, player := range team {
			lastMajor, playedSubs := playerHistory(history, player)
			found := false
			for _, sf := range candidates {
				if usedSubs[sf] {
					continue
				}
				if sf.Major == lastMajor {
					continue
				}
				if playedSubs[sf] {
					continue
				}
				teamResult[player] = sf
				usedSubs[sf] = true
				found = true
				break
			}
			if !found {
				success = false
				break
			}
		}

		if success {
			for k, v := range teamResult {
				result[k] = v
			}
			return true
		}
	}
	return false
}

// assignPositions picks random contiguous positions from sorted starts.
// Returns playerAssignment slice for all players.
func assignPositions(team1, team2 []string, numStarts int) ([]playerAssignment, error) {
	totalPlayers := len(team1) + len(team2)
	if numStarts < totalPlayers {
		return nil, fmt.Errorf("map has %d starts but need %d players", numStarts, totalPlayers)
	}

	// Pick a random starting offset for a contiguous window
	maxOffset := numStarts - totalPlayers
	offset := 0
	if maxOffset > 0 {
		offset = rand.Intn(maxOffset + 1)
	}

	// Randomly decide which team gets the first block
	team1First := rand.Intn(2) == 0

	// Shuffle within each team
	t1 := make([]string, len(team1))
	copy(t1, team1)
	rand.Shuffle(len(t1), func(i, j int) { t1[i], t1[j] = t1[j], t1[i] })

	t2 := make([]string, len(team2))
	copy(t2, team2)
	rand.Shuffle(len(t2), func(i, j int) { t2[i], t2[j] = t2[j], t2[i] })

	var assignments []playerAssignment
	var firstTeam, secondTeam []string
	var firstNum, secondNum int

	if team1First {
		firstTeam, secondTeam = t1, t2
		firstNum, secondNum = 1, 2
	} else {
		firstTeam, secondTeam = t2, t1
		firstNum, secondNum = 2, 1
	}

	for i, name := range firstTeam {
		assignments = append(assignments, playerAssignment{
			Name:       name,
			Team:       firstNum,
			StartIndex: offset + i,
		})
	}
	for i, name := range secondTeam {
		assignments = append(assignments, playerAssignment{
			Name:       name,
			Team:       secondNum,
			StartIndex: offset + len(firstTeam) + i,
		})
	}

	return assignments, nil
}
