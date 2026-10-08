package main

import "testing"

func TestWatchHelpers(t *testing.T) {
	if allOver([]float64{95, 99, 91, 92, 93}, 90) {
		t.Error("needs a full minute of samples")
	}
	if !allOver([]float64{95, 99, 91, 92, 93, 97}, 90) {
		t.Error("a minute over the limit")
	}
	if allOver([]float64{95, 99, 91, 50, 93, 97}, 90) {
		t.Error("one dip should reset")
	}
	if n, s := topOf(map[string]uint64{"a": 1, "chrome": 5, "b": 3}); n != "chrome" || s != 5 {
		t.Errorf("topOf = %s %d", n, s)
	}
}
