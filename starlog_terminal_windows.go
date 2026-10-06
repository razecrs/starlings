//go:build windows

package starlings

import (
	"encoding/binary"
	"os"
	"sync"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var readConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

func prepareStarlogTerminal(fd int) bool {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	const enableVirtualTerminalProcessing = 0x0004
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|enableVirtualTerminalProcessing) == nil
}

// prepareStarlogInteraction disables the legacy Quick Edit input mode while
// the dashboard is active. In the classic Windows console, selecting with the
// mouse otherwise suspends screen updates—and can look like the bot froze.
// The original mode is restored exactly once when Starlog closes.
func prepareStarlogInteraction() func() {
	handle := windows.Handle(os.Stdin.Fd())
	var original uint32
	if err := windows.GetConsoleMode(handle, &original); err != nil {
		return func() {}
	}
	const (
		enableProcessedInput = 0x0001
		enableLineInput      = 0x0002
		enableEchoInput      = 0x0004
		enableWindowInput    = 0x0008
		enableMouseInput     = 0x0010
		enableExtendedFlags  = 0x0080
		enableQuickEdit      = 0x0040
		enableVTInput        = 0x0200
	)
	mode := (original | enableProcessedInput | enableWindowInput | enableMouseInput | enableExtendedFlags | enableVTInput) &^
		(enableQuickEdit | enableLineInput | enableEchoInput)
	if err := windows.SetConsoleMode(handle, mode); err != nil {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() { _ = windows.SetConsoleMode(handle, original) })
	}
}

func readStarlogInput(fd int, destination []byte) int {
	handle := windows.Handle(fd)
	var events uint32
	if windows.GetNumberOfConsoleInputEvents(handle, &events) != nil || events == 0 {
		return 0
	}
	written := 0
	for events > 0 && written < len(destination) {
		var record [20]byte
		var read uint32
		result, _, _ := readConsoleInputW.Call(
			uintptr(handle), uintptr(unsafe.Pointer(&record[0])), 1, uintptr(unsafe.Pointer(&read)))
		if result == 0 || read == 0 {
			break
		}
		events--
		switch binary.LittleEndian.Uint16(record[0:2]) {
		case 0x0001: // KEY_EVENT
			if binary.LittleEndian.Uint32(record[4:8]) == 0 { // key-up
				continue
			}
			character := rune(binary.LittleEndian.Uint16(record[14:16]))
			if character != 0 {
				var encoded [utf8.UTFMax]byte
				n := utf8.EncodeRune(encoded[:], character)
				if written+n <= len(destination) {
					copy(destination[written:], encoded[:n])
					written += n
				}
				continue
			}
			sequence := ""
			switch binary.LittleEndian.Uint16(record[10:12]) {
			case 0x26:
				sequence = "\x1b[A"
			case 0x28:
				sequence = "\x1b[B"
			case 0x21:
				sequence = "\x1b[5~"
			case 0x22:
				sequence = "\x1b[6~"
			case 0x24:
				sequence = "\x1b[H"
			case 0x23:
				sequence = "\x1b[F"
			}
			if written+len(sequence) <= len(destination) {
				copy(destination[written:], sequence)
				written += len(sequence)
			}
		case 0x0002: // MOUSE_EVENT
			if binary.LittleEndian.Uint32(record[16:20]) != 0x0004 { // MOUSE_WHEELED
				continue
			}
			delta := int16(binary.LittleEndian.Uint32(record[8:12]) >> 16)
			sequence := "\x1b[<65;0;0M"
			if delta > 0 {
				sequence = "\x1b[<64;0;0M"
			}
			if written+len(sequence) <= len(destination) {
				copy(destination[written:], sequence)
				written += len(sequence)
			}
		}
	}
	return written
}

func interruptStarlogProcess() { _ = windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0) }
