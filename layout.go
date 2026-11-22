package main

// Keypad orientation: maps a key's position as the pad sits on the desk (row r, column c in an
// R×C grid, row-major from the top-left) to the pad's internal key id (1-based). Many CH57x pads
// number their keys in a rotated order.

var rotations = []int{0, 90, 180, 270}

func keyDeviceID(R, C, rot, r, c int) int {
	switch rot {
	case 90:
		return c*R + (R - 1 - r) + 1
	case 180:
		return (R-1-r)*C + (C - 1 - c) + 1
	case 270:
		return (C-1-c)*R + r + 1
	}
	return r*C + c + 1
}

// detectRotation finds the rotation where the top-left key has id tl and the top-right key has id tr.
func detectRotation(R, C, tl, tr int) (int, bool) {
	for _, rot := range rotations {
		if keyDeviceID(R, C, rot, 0, 0) == tl && keyDeviceID(R, C, rot, 0, C-1) == tr {
			return rot, true
		}
	}
	return 0, false
}

// factoryKeyID: factory-programmed CH57x pads send a, b, c… in key-id order.
func factoryKeyID(vk uint16, mods uint32) (int, bool) {
	if mods == 0 && vk >= 'A' && vk <= 'Z' {
		return int(vk-'A') + 1, true
	}
	return 0, false
}
