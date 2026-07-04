package main

import (
	"encoding/binary"
	"math"
	"testing"
)

// chunkWriter builds DataChunk-format bytes with an accompanying symbol table,
// mirroring the layout ChunkReader.Next expects: id(u32) version(u16) size(i32) payload.
type chunkWriter struct {
	tbl    SymbolTable
	nextID uint32
	ids    map[string]uint32
}

func newChunkWriter() *chunkWriter {
	return &chunkWriter{tbl: SymbolTable{}, nextID: 1, ids: map[string]uint32{}}
}

func (w *chunkWriter) idFor(name string) uint32 {
	if id, ok := w.ids[name]; ok {
		return id
	}
	id := w.nextID
	w.nextID++
	w.ids[name] = id
	w.tbl[id] = name
	return id
}

func (w *chunkWriter) chunk(name string, version uint16, payload []byte) []byte {
	out := make([]byte, 0, 10+len(payload))
	out = binary.LittleEndian.AppendUint32(out, w.idFor(name))
	out = binary.LittleEndian.AppendUint16(out, version)
	out = binary.LittleEndian.AppendUint32(out, uint32(int32(len(payload))))
	return append(out, payload...)
}

func i32(v int32) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }
func f32(v float32) []byte {
	return binary.LittleEndian.AppendUint32(nil, math.Float32bits(v))
}
func astr(s string) []byte {
	out := binary.LittleEndian.AppendUint16(nil, uint16(len(s)))
	return append(out, []byte(s)...)
}

// param writes a non-COORD3D script Parameter.
func param(paramType, intVal int32, str string) []byte {
	b := i32(paramType)
	b = append(b, i32(intVal)...)
	b = append(b, f32(0)...)
	b = append(b, astr(str)...)
	return b
}

// buildScript assembles a full Script chunk (version 2) with an optional
// always-true condition and a WAREHOUSE_SET_VALUE action.
func (w *chunkWriter) buildScript(isActive byte, condType int32, whName string, cash int32, actionType int32) []byte {
	// Condition -> OrCondition
	cond := i32(condType) // conditionType
	cond = append(cond, i32(0)...)
	condChunk := w.chunk("Condition", 1, cond)
	orChunk := w.chunk("OrCondition", 1, condChunk)

	// ScriptAction: actionType, numParms=2, [name param, cash param]
	act := i32(actionType)
	act = append(act, i32(2)...)
	act = append(act, param(0, 0, whName)...)
	act = append(act, param(0, cash, "")...)
	actChunk := w.chunk("ScriptAction", 1, act)

	// Script payload
	p := astr("scriptName")
	p = append(p, astr("")...) // comment
	p = append(p, astr("")...) // conditionComment
	p = append(p, astr("")...) // actionComment
	p = append(p, isActive)    // isActive
	p = append(p, 0, 1, 1, 1)  // isOneShot, easy, normal, hard
	p = append(p, 0)           // isSubroutine
	p = append(p, i32(0)...)   // delayEvaluationSeconds (version 2)
	p = append(p, orChunk...)
	p = append(p, actChunk...)
	return w.chunk("Script", 2, p)
}

func (w *chunkWriter) playerScriptsList(scripts ...[]byte) *Chunk {
	var list []byte
	for _, s := range scripts {
		list = append(list, s...)
	}
	slChunk := w.chunk("ScriptList", 1, list)
	pslBytes := w.chunk("PlayerScriptsList", 1, slChunk)
	// Re-read the PlayerScriptsList chunk into a *Chunk via ChunkReader.
	r := NewChunkReader(pslBytes, 0, len(pslBytes), w.tbl)
	c, err := r.Next()
	if err != nil {
		panic(err)
	}
	return c
}

func TestWarehouseOverrideStartOfGame(t *testing.T) {
	w := newChunkWriter()
	script := w.buildScript(1, conditionTrue, "MyWarehouse", 15000, actionWarehouseSetValue)
	psl := w.playerScriptsList(script)

	md := &MapData{}
	parsePlayerScriptsList(psl, w.tbl, md)

	if got := md.SupplyOverrides["MyWarehouse"]; got != 15000 {
		t.Fatalf("override = %d, want 15000 (overrides=%v)", got, md.SupplyOverrides)
	}
}

func TestWarehouseOverrideInactiveSkipped(t *testing.T) {
	w := newChunkWriter()
	script := w.buildScript(0, conditionTrue, "MyWarehouse", 15000, actionWarehouseSetValue) // inactive
	psl := w.playerScriptsList(script)

	md := &MapData{}
	parsePlayerScriptsList(psl, w.tbl, md)

	if _, ok := md.SupplyOverrides["MyWarehouse"]; ok {
		t.Fatalf("inactive script should not produce an override, got %v", md.SupplyOverrides)
	}
}

func TestWarehouseOverrideConditionalSkipped(t *testing.T) {
	w := newChunkWriter()
	// Active, but condition is not CONDITION_TRUE -> conditional/later, must skip.
	script := w.buildScript(1, conditionTrue+1, "MyWarehouse", 15000, actionWarehouseSetValue)
	psl := w.playerScriptsList(script)

	md := &MapData{}
	parsePlayerScriptsList(psl, w.tbl, md)

	if _, ok := md.SupplyOverrides["MyWarehouse"]; ok {
		t.Fatalf("conditional script should not produce an override, got %v", md.SupplyOverrides)
	}
}

func TestNonWarehouseActionIgnored(t *testing.T) {
	w := newChunkWriter()
	script := w.buildScript(1, conditionTrue, "MyWarehouse", 15000, actionWarehouseSetValue+1) // different action
	psl := w.playerScriptsList(script)

	md := &MapData{}
	parsePlayerScriptsList(psl, w.tbl, md)

	if len(md.SupplyOverrides) != 0 {
		t.Fatalf("non-warehouse action should be ignored, got %v", md.SupplyOverrides)
	}
}

func TestResolveSupplyAmount(t *testing.T) {
	md := &MapData{SupplyOverrides: map[string]int{"Named1": 12345}}

	// Override wins.
	amt, over, known := resolveSupplyAmount(md, &MapObject{Name: "SupplyWarehouse", EditorName: "Named1"})
	if amt != 12345 || !over || !known {
		t.Fatalf("override case: amt=%d over=%v known=%v", amt, over, known)
	}

	// INI default when no override.
	amt, over, known = resolveSupplyAmount(md, &MapObject{Name: "SupplyPile"})
	if amt != 150*valuePerSupplyBox || over || !known {
		t.Fatalf("default case: amt=%d over=%v known=%v", amt, over, known)
	}

	// Unknown template.
	amt, over, known = resolveSupplyAmount(md, &MapObject{Name: "NotASupply"})
	if amt != 0 || over || known {
		t.Fatalf("unknown case: amt=%d over=%v known=%v", amt, over, known)
	}
}
