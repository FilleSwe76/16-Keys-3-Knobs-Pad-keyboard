package main

import "testing"

// A factory-programmed 514C:8850 pad (4×4 + 3 knobs) types these letters when its keys are
// pressed row by row as it sits on the desk. Key id n sends letter 'a'+n-1.
var factoryRows = []string{"dhlp", "cgko", "bfjn", "aeim"}

func TestDetectRotationFromFactoryLetters(t *testing.T) {
	tl, _ := factoryKeyID(uint16('D'), 0)
	tr, _ := factoryKeyID(uint16('P'), 0)
	rot, ok := detectRotation(4, 4, tl, tr)
	if !ok || rot != 90 {
		t.Fatalf("got rotation %d ok=%v, want 90", rot, ok)
	}
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			id := keyDeviceID(4, 4, rot, r, c)
			if got := byte('a' + id - 1); got != factoryRows[r][c] {
				t.Errorf("row %d col %d: id %d = %c, want %c", r, c, id, got, factoryRows[r][c])
			}
		}
	}
}

func TestRotationsArePermutations(t *testing.T) {
	for _, dims := range [][2]int{{4, 4}, {3, 4}, {2, 3}, {1, 5}} {
		R, C := dims[0], dims[1]
		for _, rot := range rotations {
			seen := map[int]bool{}
			for r := 0; r < R; r++ {
				for c := 0; c < C; c++ {
					id := keyDeviceID(R, C, rot, r, c)
					if id < 1 || id > R*C || seen[id] {
						t.Fatalf("%dx%d rot %d: bad or duplicate id %d", R, C, rot, id)
					}
					seen[id] = true
				}
			}
		}
	}
}
