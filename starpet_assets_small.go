//go:build starlings_small

package starlings

// Keep the terminal-native animated pets and custom sprite support while
// omitting the bundled PNG sheets from size-sensitive builds.
var starPetLumaPNG, starPetCometPNG, starPetNebulaPNG []byte
