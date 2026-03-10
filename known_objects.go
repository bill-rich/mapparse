package main

// Supply source objects (KINDOF_SUPPLY_SOURCE_ON_PREVIEW).
// From inizh/Data/INI/Object/CivilianBuilding.ini
var supplyNames = map[string]bool{
	"SupplyWarehouse": true,
	"SupplyDock":      true,
	"SupplyPile":      true,
	"SupplyPileSmall": true,
	"ToxinRepository": true,
}

// Tech building objects (KINDOF_TECH_BUILDING).
// From inizh/Data/INI/Object/TechBuildings.ini and CivilianBuilding.ini
var techNames = map[string]bool{
	"TechArtilleryPlatform": true,
	"TechReinforcementPad":  true,
	"TechRepairPad":         true,
	"TechRepairbay":         true,
	"TechHospital":          true,
	"TechOilDerrick":        true,
	"TechOilRefinery":       true,
}
