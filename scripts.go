package main

import "fmt"

// Script enum ordinals, serialized as raw ints in the map's script chunks.
// These match both Generals and Zero Hour (verified against Scripts.h in both
// GeneralsGameCode trees; the ordinals are identical and never reordered, since
// doing so would break map compatibility).
const (
	actionWarehouseSetValue = 212 // ScriptActionType::WAREHOUSE_SET_VALUE
	conditionTrue           = 3   // ConditionType::CONDITION_TRUE
	paramCoord3D            = 16  // Parameter::COORD3D
)

// Script chunk versions that gate optional trailing fields.
const (
	scriptDataVersion2      = 2 // adds delayEvaluationSeconds
	scriptGroupDataVersion2 = 2 // adds isGroupSubroutine
)

// parseSidesList walks the SidesList chunk to reach its nested PlayerScriptsList
// and records any start-of-game "Set Warehouse Value" overrides into
// md.SupplyOverrides (keyed by the target object's editor name).
//
// Overrides are best-effort: the fixed SidesList fields must be parsed exactly
// to locate the script sub-chunk, so a format surprise returns an error that the
// caller treats as non-fatal (core object output is unaffected).
func parseSidesList(chunk *Chunk, tbl SymbolTable, md *MapData) error {
	r := NewBinReader(chunk.Data)

	numSides, err := r.ReadInt32()
	if err != nil {
		return fmt.Errorf("numSides: %w", err)
	}
	for i := int32(0); i < numSides; i++ {
		if _, err := r.ReadDict(tbl); err != nil {
			return fmt.Errorf("side %d dict: %w", i, err)
		}
		numBuild, err := r.ReadInt32()
		if err != nil {
			return fmt.Errorf("side %d buildCount: %w", i, err)
		}
		for j := int32(0); j < numBuild; j++ {
			if err := skipBuildListEntry(r, chunk.Version); err != nil {
				return fmt.Errorf("side %d build %d: %w", i, j, err)
			}
		}
	}

	// K_SIDES_DATA_VERSION_2 and up append the team list.
	if chunk.Version >= 2 {
		numTeams, err := r.ReadInt32()
		if err != nil {
			return fmt.Errorf("teamCount: %w", err)
		}
		for i := int32(0); i < numTeams; i++ {
			if _, err := r.ReadDict(tbl); err != nil {
				return fmt.Errorf("team %d dict: %w", i, err)
			}
		}
	}

	// Whatever remains is the nested PlayerScriptsList chunk.
	rest := r.Rest()
	sub := NewChunkReader(rest, 0, len(rest), tbl)
	for {
		c, err := sub.Next()
		if err != nil {
			return fmt.Errorf("scripts sub-chunk: %w", err)
		}
		if c == nil {
			break
		}
		if c.Name == "PlayerScriptsList" {
			parsePlayerScriptsList(c, tbl, md)
		}
	}
	return nil
}

// skipBuildListEntry consumes one BuildListInfo record from the SidesList payload.
func skipBuildListEntry(r *BinReader, version uint16) error {
	if _, err := r.ReadAsciiString(); err != nil { // buildingName
		return err
	}
	if _, err := r.ReadAsciiString(); err != nil { // templateName
		return err
	}
	for i := 0; i < 4; i++ { // x, y, z, angle
		if _, err := r.ReadFloat32(); err != nil {
			return err
		}
	}
	if _, err := r.ReadByte(); err != nil { // initiallyBuilt
		return err
	}
	if _, err := r.ReadInt32(); err != nil { // numRebuilds
		return err
	}
	// K_SIDES_DATA_VERSION_3 appends per-entry script/health/flags.
	if version >= 3 {
		if _, err := r.ReadAsciiString(); err != nil { // script
			return err
		}
		if _, err := r.ReadInt32(); err != nil { // health
			return err
		}
		for i := 0; i < 3; i++ { // whiner, unsellable, repairable
			if _, err := r.ReadByte(); err != nil {
				return err
			}
		}
	}
	return nil
}

// parsePlayerScriptsList iterates the per-player ScriptList sub-chunks.
func parsePlayerScriptsList(chunk *Chunk, tbl SymbolTable, md *MapData) {
	sub := SubReader(chunk, tbl)
	for {
		c, err := sub.Next()
		if err != nil || c == nil {
			return
		}
		if c.Name == "ScriptList" {
			parseScriptList(c, tbl, md)
		}
	}
}

// parseScriptList iterates the Script and ScriptGroup sub-chunks of one player's
// script list. Group activeness gates the scripts it contains.
func parseScriptList(chunk *Chunk, tbl SymbolTable, md *MapData) {
	sub := SubReader(chunk, tbl)
	for {
		c, err := sub.Next()
		if err != nil || c == nil {
			return
		}
		switch c.Name {
		case "Script":
			parseScript(c, tbl, md, true)
		case "ScriptGroup":
			parseScriptGroup(c, tbl, md)
		}
	}
}

// parseScriptGroup reads a group's active flag and recurses into its scripts.
func parseScriptGroup(chunk *Chunk, tbl SymbolTable, md *MapData) {
	r := NewBinReader(chunk.Data)
	if _, err := r.ReadAsciiString(); err != nil { // groupName
		return
	}
	active, err := r.ReadByte()
	if err != nil {
		return
	}
	if chunk.Version >= scriptGroupDataVersion2 {
		if _, err := r.ReadByte(); err != nil { // isGroupSubroutine
			return
		}
	}
	groupActive := active != 0

	rest := r.Rest()
	sub := NewChunkReader(rest, 0, len(rest), tbl)
	for {
		c, err := sub.Next()
		if err != nil || c == nil {
			return
		}
		if c.Name == "Script" {
			parseScript(c, tbl, md, groupActive)
		}
	}
}

// parseScript reads one Script chunk. When the script is active (and its
// enclosing group, if any) and its condition is unconditionally true, any
// "Set Warehouse Value" actions it performs are recorded as start-of-game
// supply overrides. Conditional/later scripts are ignored per the tool's scope.
func parseScript(chunk *Chunk, tbl SymbolTable, md *MapData, enclosingActive bool) {
	r := NewBinReader(chunk.Data)
	for i := 0; i < 4; i++ { // scriptName, comment, conditionComment, actionComment
		if _, err := r.ReadAsciiString(); err != nil {
			return
		}
	}
	active, err := r.ReadByte()
	if err != nil {
		return
	}
	// isOneShot, easy, normal, hard, isSubroutine
	for i := 0; i < 5; i++ {
		if _, err := r.ReadByte(); err != nil {
			return
		}
	}
	if chunk.Version >= scriptDataVersion2 {
		if _, err := r.ReadInt32(); err != nil { // delayEvaluationSeconds
			return
		}
	}

	scriptActive := enclosingActive && active != 0

	rest := r.Rest()
	sub := NewChunkReader(rest, 0, len(rest), tbl)

	firesAtStart := false
	type whSet struct {
		name string
		cash int
	}
	var sets []whSet

	for {
		c, err := sub.Next()
		if err != nil || c == nil {
			break
		}
		switch c.Name {
		case "OrCondition":
			if orConditionAlwaysTrue(c, tbl) {
				firesAtStart = true
			}
		case "ScriptAction":
			if name, cash, ok := warehouseSetFromAction(c); ok {
				sets = append(sets, whSet{name, cash})
			}
		}
	}

	if scriptActive && firesAtStart {
		for _, s := range sets {
			if s.name == "" {
				continue
			}
			if md.SupplyOverrides == nil {
				md.SupplyOverrides = make(map[string]int)
			}
			md.SupplyOverrides[s.name] = s.cash
		}
	}
}

// orConditionAlwaysTrue reports whether an OR branch is unconditionally true:
// it contains at least one Condition and every Condition is CONDITION_TRUE.
func orConditionAlwaysTrue(chunk *Chunk, tbl SymbolTable) bool {
	sub := SubReader(chunk, tbl)
	sawCondition := false
	for {
		c, err := sub.Next()
		if err != nil || c == nil {
			break
		}
		if c.Name != "Condition" {
			continue
		}
		sawCondition = true
		ct, ok := conditionType(c)
		if !ok || ct != conditionTrue {
			return false
		}
	}
	return sawCondition
}

// conditionType reads the leading conditionType int of a Condition chunk.
func conditionType(chunk *Chunk) (int32, bool) {
	r := NewBinReader(chunk.Data)
	ct, err := r.ReadInt32()
	if err != nil {
		return 0, false
	}
	return ct, true
}

// warehouseSetFromAction returns the warehouse name and cash value if the action
// is WAREHOUSE_SET_VALUE. Params: 0 = warehouse unit name, 1 = cash value.
func warehouseSetFromAction(chunk *Chunk) (string, int, bool) {
	r := NewBinReader(chunk.Data)
	actionType, err := r.ReadInt32()
	if err != nil || actionType != actionWarehouseSetValue {
		return "", 0, false
	}
	numParms, err := r.ReadInt32()
	if err != nil || numParms < 2 {
		return "", 0, false
	}
	params := make([]scriptParam, 0, numParms)
	for i := int32(0); i < numParms; i++ {
		p, err := readScriptParam(r)
		if err != nil {
			return "", 0, false
		}
		params = append(params, p)
	}
	return params[0].str, int(params[1].intVal), true
}

// scriptParam holds the fields of a parsed script Parameter that we care about.
type scriptParam struct {
	intVal int32
	str    string
}

// readScriptParam consumes one Parameter record (see Parameter::ReadParameter).
func readScriptParam(r *BinReader) (scriptParam, error) {
	paramType, err := r.ReadInt32()
	if err != nil {
		return scriptParam{}, err
	}
	if paramType == paramCoord3D {
		for i := 0; i < 3; i++ { // x, y, z
			if _, err := r.ReadFloat32(); err != nil {
				return scriptParam{}, err
			}
		}
		return scriptParam{}, nil
	}
	iv, err := r.ReadInt32()
	if err != nil {
		return scriptParam{}, err
	}
	if _, err := r.ReadFloat32(); err != nil { // m_real
		return scriptParam{}, err
	}
	s, err := r.ReadAsciiString()
	if err != nil {
		return scriptParam{}, err
	}
	return scriptParam{intVal: iv, str: s}, nil
}
