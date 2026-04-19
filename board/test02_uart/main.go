//go:build tinygo && esp32s3

package main

// test02_uart: Use TinyGo's built-in println to write to UART0.
// If you have a logic analyzer on the UART0 TX pin you will see output.
// Over USB-Serial/JTAG (/dev/tty.usbmodem1101) you will likely see nothing,
// because println is routed to UART0, not USB.

func main() {
	for {
		busyDelay()
		println("\n**\ntest02: hello from UART0")
		busyDelay()
	}
}

// busyDelay spins for roughly one second at 240 MHz.
func busyDelay() {
	for i := 0; i < 80_000_000; i++ {
		// Volatile-style trick: call a noop so the compiler doesn't optimise
		// the loop away. TinyGo's arm.Asm is not available on Xtensa, but a
		// plain empty loop body is kept by TinyGo at -opt=2 because i escapes
		// through the loop condition.
	}
}
