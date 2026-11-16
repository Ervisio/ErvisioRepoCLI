//go:build darwin || linux

package iostreams

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct{ row, col, xpixel, ypixel uint16 }

func termWidth(f *os.File) int {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws))) //nolint:gosec // ioctl needs the struct address
	if errno != 0 {
		return 0
	}
	return int(ws.col)
}

// disableEcho turns off terminal echo and returns the function that
// restores the previous state.
func disableEcho(f *os.File) (func(), error) {
	var old syscall.Termios
	fd := f.Fd()
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(&old))); errno != 0 { //nolint:gosec // ioctl needs the struct address
		return nil, errno
	}
	t := old
	t.Lflag &^= syscall.ECHO
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&t))); errno != 0 { //nolint:gosec // ioctl needs the struct address
		return nil, errno
	}
	return func() {
		syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&old))) //nolint:errcheck,gosec // best effort restore
	}, nil
}
