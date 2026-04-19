//go:build tinygo && esp32s3

package main

// test01_blink: absolute minimum — just an infinite empty loop.
// Tests that the binary flashes and boots without crashing.
// Verify: the USB port (/dev/tty.usbmodem1101) stays stable and
// does not keep re-enumerating.

func main() {
	for {
	}
}
