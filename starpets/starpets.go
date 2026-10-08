// Package starpets adds Starlings' bundled pixel art to Starlog's pets.
// Import it for its side effect:
//
//	import _ "github.com/razecrs/starlings/starpets"
//
// The art adds about 5 MB to a binary, so it lives in its own package. Without
// it, NewStarPet draws the same pets with terminal characters.
package starpets

import (
	_ "embed"

	"github.com/razecrs/starlings"
)

var (
	//go:embed luma.png
	luma []byte
	//go:embed comet.png
	comet []byte
	//go:embed nebula.png
	nebula []byte
)

func init() {
	starlings.RegisterStarPetArt(starlings.StarPetNova, luma)
	starlings.RegisterStarPetArt(starlings.StarPetComet, comet)
	starlings.RegisterStarPetArt(starlings.StarPetNebula, nebula)
}
