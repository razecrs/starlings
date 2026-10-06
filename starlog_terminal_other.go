//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package starlings

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func prepareStarlogTerminal(fd int) bool { return term.IsTerminal(fd) }

func prepareStarlogInteraction() func() {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return func() {}
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(func() { _ = term.Restore(fd, state) }) }
}

func readStarlogInput(fd int, destination []byte) int {
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	ready, err := unix.Poll(poll, 0)
	if err != nil || ready == 0 || poll[0].Revents&unix.POLLIN == 0 {
		return 0
	}
	n, err := unix.Read(fd, destination)
	if err != nil {
		return 0
	}
	return n
}

func interruptStarlogProcess() { _ = unix.Kill(os.Getpid(), unix.SIGINT) }
