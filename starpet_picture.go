package starlings

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"strings"
	"sync"
	"time"
)

// StarlogSpriteSheet describes pictured Star Pet animation frames. PNG remains
// portable and embeddable; Starlog decodes it once and renders it directly to
// true-colour terminal cells without an external image process.
type StarlogSpriteSheet struct {
	PNG     []byte
	Columns int
	Rows    int
	Width   int // preferred terminal width in cells

	Calm  []int
	Build []int
	Warn  []int
	Error []int

	once   sync.Once
	frames []*image.NRGBA // one small image per cell; nil when the cell is empty
	err    error

	renderMu sync.RWMutex
	rendered map[starlogRenderedFrame]StarlogFrame
}

// starlogFrameDetail is how many source pixels each frame keeps per terminal
// cell of the preferred width. Rendering samples at most two pixels per cell,
// so four leaves headroom without holding the full-resolution sheet: a
// 1448x1086 sheet would otherwise stay resident as about 6 MB of RGBA.
const starlogFrameDetail = 4

type starlogRenderedFrame struct {
	frame int
	width int
}

// NewStarlogSpriteSheet creates a custom pictured-pet sheet. Frames are
// numbered left-to-right, top-to-bottom. The byte slice is copied.
func NewStarlogSpriteSheet(png []byte, columns, rows, width int) *StarlogSpriteSheet {
	return &StarlogSpriteSheet{
		PNG:     append([]byte(nil), png...),
		Columns: columns,
		Rows:    rows,
		Width:   width,
	}
}

var starPetArt struct {
	mu  sync.RWMutex
	png [3][]byte
}

// RegisterStarPetArt sets the pixel-art sheet NewStarPet uses for variant.
// The starpets package registers Starlings' bundled art when it is imported;
// without it, pets draw with terminal characters only. The slice is shared,
// not copied, so the caller must not modify it afterwards.
func RegisterStarPetArt(variant StarPetVariant, png []byte) {
	if int(variant) >= len(starPetArt.png) {
		return
	}
	starPetArt.mu.Lock()
	starPetArt.png[variant] = png
	starPetArt.mu.Unlock()
}

// newBuiltInSpriteSheet shares the registered PNG instead of copying it, so
// every pet built from the same variant costs no extra image memory.
func newBuiltInSpriteSheet(variant StarPetVariant) *StarlogSpriteSheet {
	if int(variant) >= len(starPetArt.png) {
		return nil
	}
	starPetArt.mu.RLock()
	png := starPetArt.png[variant]
	starPetArt.mu.RUnlock()
	if len(png) == 0 {
		return nil
	}
	return &StarlogSpriteSheet{
		PNG:     png,
		Columns: 4,
		Rows:    3,
		Width:   22,
		Calm:    []int{0, 1, 2, 3},
		Build:   []int{4, 5, 6, 7},
		Warn:    []int{8, 9},
		Error:   []int{10, 11},
	}
}

// prepare decodes the sheet once, crops each cell to its visible pixels, and
// keeps a small copy of every frame. The decoded sheet is released afterwards.
func (s *StarlogSpriteSheet) prepare() error {
	if s == nil {
		return fmt.Errorf("starlings: nil Starlog sprite sheet")
	}
	s.once.Do(func() {
		if s.Columns <= 0 || s.Rows <= 0 {
			s.err = fmt.Errorf("starlings: sprite sheet needs columns and rows")
			return
		}
		source, _, err := image.Decode(bytes.NewReader(s.PNG))
		if err != nil {
			s.err = err
			return
		}
		width := s.Width
		if width <= 0 {
			width = 22
		}
		maxWidth := width * starlogFrameDetail
		bounds := source.Bounds()
		cellWidth, cellHeight := bounds.Dx()/s.Columns, bounds.Dy()/s.Rows
		s.frames = make([]*image.NRGBA, s.Columns*s.Rows)
		for index := range s.frames {
			x, y := index%s.Columns, index/s.Columns
			cell := image.Rect(
				bounds.Min.X+x*cellWidth,
				bounds.Min.Y+y*cellHeight,
				bounds.Min.X+(x+1)*cellWidth,
				bounds.Min.Y+(y+1)*cellHeight,
			)
			visible := cropVisibleBounds(source, cell)
			if visible.Empty() {
				continue
			}
			s.frames[index] = shrinkFrame(source, visible, maxWidth)
		}
	})
	return s.err
}

// shrinkFrame copies area into a new image no wider than maxWidth, keeping
// the aspect ratio. Colours are stored un-premultiplied, as samplePicture
// expects.
func shrinkFrame(source image.Image, area image.Rectangle, maxWidth int) *image.NRGBA {
	width := min(area.Dx(), maxWidth)
	height := max(1, area.Dy()*width/max(1, area.Dx()))
	small := image.NewNRGBA(image.Rect(0, 0, width, height))
	sub := &starlogSubImage{source: source, bounds: area}
	for y := range height {
		for x := range width {
			small.SetNRGBA(x, y, color.NRGBA(samplePicture(sub, x, y, width, height)))
		}
	}
	return small
}

func (s *StarlogSpriteSheet) frame(mood StarlogMood, now time.Time, delay time.Duration) (image.Image, int, bool) {
	if err := s.prepare(); err != nil {
		return nil, 0, false
	}
	frames := s.Calm
	switch mood {
	case StarlogBuild:
		frames = s.Build
	case StarlogWarn:
		frames = s.Warn
	case StarlogError:
		frames = s.Error
	}
	if len(frames) == 0 {
		return nil, 0, false
	}
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}
	framePosition := int(now.UnixNano()/int64(delay)) % len(frames)
	if mood == StarlogCalm {
		// Idle pets have personality without constantly dancing: a blink, a
		// glance/tail twitch, and one happy pose are staggered across a long
		// cycle. petCrewFrame offsets each pet so the crew never moves in lockstep.
		framePosition = 0
		idleCycle := 9 * time.Second
		phase := time.Duration(now.UnixNano() % int64(idleCycle))
		switch {
		case len(frames) > 1 && phase >= 2500*time.Millisecond && phase < 2780*time.Millisecond:
			framePosition = 1
		case len(frames) > 2 && phase >= 5100*time.Millisecond && phase < 5700*time.Millisecond:
			framePosition = 2
		case len(frames) > 3 && phase >= 7600*time.Millisecond && phase < 8250*time.Millisecond:
			framePosition = 3
		}
	}
	frame := frames[framePosition]
	if frame < 0 || frame >= s.Columns*s.Rows {
		return nil, 0, false
	}
	if s.frames[frame] == nil {
		return nil, 0, false
	}
	return s.frames[frame], frame, true
}

func cropVisibleBounds(source image.Image, cell image.Rectangle) image.Rectangle {
	visible := image.Rectangle{}
	found := false
	for y := cell.Min.Y; y < cell.Max.Y; y++ {
		for x := cell.Min.X; x < cell.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha < 0x1000 {
				continue
			}
			if !found {
				visible = image.Rect(x, y, x+1, y+1)
				found = true
			} else {
				visible = visible.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if !found {
		return image.Rectangle{}
	}
	padding := max(2, min(visible.Dx(), visible.Dy())/40)
	visible = image.Rect(
		max(cell.Min.X, visible.Min.X-padding),
		max(cell.Min.Y, visible.Min.Y-padding),
		min(cell.Max.X, visible.Max.X+padding),
		min(cell.Max.Y, visible.Max.Y+padding),
	)
	return visible
}

type starlogSubImage struct {
	source image.Image
	bounds image.Rectangle
}

func (s *starlogSubImage) ColorModel() color.Model { return s.source.ColorModel() }
func (s *starlogSubImage) Bounds() image.Rectangle { return s.bounds }
func (s *starlogSubImage) At(x, y int) color.Color { return s.source.At(x, y) }

func renderStarPetPicture(pet StarlogPet, mood StarlogMood, now time.Time, available int) (StarlogFrame, bool) {
	if pet.Picture == nil || available < 8 {
		return nil, false
	}
	frame, frameIndex, ok := pet.Picture.frame(mood, now, pet.FrameDelay)
	if !ok || frame == nil {
		return nil, false
	}
	width := pet.Picture.Width
	if width <= 0 {
		width = 22
	}
	width = min(width, available)
	key := starlogRenderedFrame{frame: frameIndex, width: width}
	pet.Picture.renderMu.RLock()
	if rendered, found := pet.Picture.rendered[key]; found {
		pet.Picture.renderMu.RUnlock()
		return rendered, true
	}
	pet.Picture.renderMu.RUnlock()
	bounds := frame.Bounds()
	pixelHeight := max(2, bounds.Dy()*width/max(1, bounds.Dx()))
	// One terminal cell represents two square image samples vertically.
	if pixelHeight%2 != 0 {
		pixelHeight++
	}
	lines := make(StarlogFrame, pixelHeight/2)
	for row := 0; row < pixelHeight; row += 2 {
		var line strings.Builder
		for column := 0; column < width; column++ {
			top := samplePicture(frame, column, row, width, pixelHeight)
			bottom := samplePicture(frame, column, row+1, width, pixelHeight)
			switch {
			case top.A < 20 && bottom.A < 20:
				line.WriteString("\x1b[0m ")
			case bottom.A < 20:
				fmt.Fprintf(&line, "\x1b[0m\x1b[38;2;%d;%d;%dm▀", top.R, top.G, top.B)
			case top.A < 20:
				fmt.Fprintf(&line, "\x1b[0m\x1b[38;2;%d;%d;%dm▄", bottom.R, bottom.G, bottom.B)
			default:
				fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
					top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
			}
		}
		line.WriteString(ansiReset)
		lines[row/2] = line.String()
	}
	pet.Picture.renderMu.Lock()
	if pet.Picture.rendered == nil {
		pet.Picture.rendered = make(map[starlogRenderedFrame]StarlogFrame)
	}
	if rendered, found := pet.Picture.rendered[key]; found {
		lines = rendered
	} else {
		pet.Picture.rendered[key] = lines
	}
	pet.Picture.renderMu.Unlock()
	return lines, true
}

func samplePicture(source image.Image, x, y, width, height int) color.RGBA {
	bounds := source.Bounds()
	sourceX := bounds.Min.X + min(bounds.Dx()-1, x*bounds.Dx()/width)
	sourceY := bounds.Min.Y + min(bounds.Dy()-1, y*bounds.Dy()/height)
	r, g, b, a := source.At(sourceX, sourceY).RGBA()
	if a == 0 {
		return color.RGBA{}
	}
	// RGBA returns alpha-premultiplied channels. Undo that so translucent edge
	// pixels do not acquire a dark fringe on the terminal background.
	return color.RGBA{
		R: uint8(min(uint64(0xffff), uint64(r)*0xffff/uint64(a)) >> 8),
		G: uint8(min(uint64(0xffff), uint64(g)*0xffff/uint64(a)) >> 8),
		B: uint8(min(uint64(0xffff), uint64(b)*0xffff/uint64(a)) >> 8),
		A: uint8(a >> 8),
	}
}
