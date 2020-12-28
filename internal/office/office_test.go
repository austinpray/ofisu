package office

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestFromFile(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)

	office, err := FromFile(filepath.Dir(filename) + "/../../deploy/offices/awnam.dot")

	if err != nil {
		t.Errorf("expected office to parse, but got %v", err)
	}

	if office.ID != "awnam" {
		t.Errorf("expected office id to be awnam_office but got %v", office.ID)
	}

	if len(office.Edges) != 18 {
		t.Errorf("expected office to have 18 edges but got %v", len(office.Edges))
	}

	if office.Edges["parking_lot"][0] != "reception" {
		t.Errorf("parking lot should connect to reception")
	}

	reception := office.Edges["reception"]

	seenFrizzle := false
	for _, edge := range reception {
		if edge == "frizzle_office" {
			seenFrizzle = true
		}
	}
	if !seenFrizzle {
		t.Errorf("reception should connect to frizzle office")
	}

	if office.Rooms["break"].Name != "Break Room" {
		t.Errorf("break should be named 'Break Room' but got %v", office.Rooms["break"].Name)
	}

	seenGamecube := false
	for _, item := range office.Rooms["break"].Items {
		if item == "gamecube" {
			seenGamecube = true
		}
	}
	if !seenGamecube {
		t.Errorf("break should have a gamecube but only have %v", office.Rooms["break"].Items)
	}

	if len(office.Edges["parking_lot"]) != 1 {
		t.Errorf("too many edges on parking lot")
	}

	candidates := office.GetMoveCandidates("hallway_east", "men's bathroom")
	if len(candidates) != 1 {
		t.Errorf("expected only one move candidate %v", candidates)
	}
	if candidates[0].ID != "bathroom_men" {
		t.Errorf("should have matched bathroom_men but got %v", candidates[0].ID)
	}
}
