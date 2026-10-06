//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package starlings

func prepareStarlogTerminal(_ int) bool { return false }

func prepareStarlogInteraction() func() { return func() {} }

func readStarlogInput(_ int, _ []byte) int { return 0 }

func interruptStarlogProcess() {}
