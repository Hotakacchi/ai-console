package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestPeekSpot(t *testing.T) {
	wa := application.Rect{X: 100, Y: 50, Width: 1920, Height: 1040}
	for pos, want := range map[string][3]any{
		"bottom-right":  {100 + 1920 - peekW - 12, 50 + 1040 - peekH, "bottom"},
		"bottom-left":   {112, 50 + 1040 - peekH, "bottom"},
		"bottom-center": {100 + (1920-peekW)/2, 50 + 1040 - peekH, "bottom"},
		"top-right":     {100 + 1920 - peekW - 12, 50, "top"},
		"top-left":      {112, 50, "top"},
		"top-center":    {100 + (1920-peekW)/2, 50, "top"},
	} {
		x, y, from := peekSpot(pos, wa)
		if x != want[0] || y != want[1] || from != want[2] {
			t.Errorf("%s: %d,%d %s", pos, x, y, from)
		}
	}
}
