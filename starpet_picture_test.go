package starlings

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// A mark beside the pet in one frame (a "!", a "z") must not change that
// frame's scale or position, or the pet jumps between moods.
func TestSpriteSheetCropsEveryFrameToTheSameBox(t *testing.T) {
	const cell = 16
	sheet := image.NewNRGBA(image.Rect(0, 0, 2*cell, cell))
	body := color.NRGBA{R: 40, G: 50, B: 140, A: 255}
	mark := color.NRGBA{R: 255, G: 200, B: 60, A: 255}
	for _, origin := range []int{0, cell} {
		for y := 4; y < 12; y++ {
			for x := 4; x < 12; x++ {
				sheet.SetNRGBA(origin+x, y, body)
			}
		}
	}
	sheet.SetNRGBA(cell+0, 0, mark) // only the second frame has it

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, sheet); err != nil {
		t.Fatal(err)
	}
	s := NewStarlogSpriteSheet(encoded.Bytes(), 2, 1, cell)
	if err := s.prepare(); err != nil {
		t.Fatal(err)
	}
	plain, marked := s.frames[0], s.frames[1]
	if plain == nil || marked == nil {
		t.Fatal("a frame was dropped")
	}
	if plain.Bounds() != marked.Bounds() {
		t.Fatalf("frames differ in size: %v and %v", plain.Bounds(), marked.Bounds())
	}
	for y := plain.Bounds().Min.Y; y < plain.Bounds().Max.Y; y++ {
		for x := plain.Bounds().Min.X; x < plain.Bounds().Max.X; x++ {
			if plain.NRGBAAt(x, y) == body && marked.NRGBAAt(x, y) != body {
				t.Fatalf("the body moved between frames at (%d, %d)", x, y)
			}
		}
	}
}
