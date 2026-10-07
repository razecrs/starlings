//go:build !starlings_small

package starlings

import _ "embed"

//go:embed assets/starpet-luma.png
var starPetLumaPNG []byte

//go:embed assets/starpet-comet.png
var starPetCometPNG []byte

//go:embed assets/starpet-nebula.png
var starPetNebulaPNG []byte
