//go:build tinygo && esp32s3

package main

import "unsafe"

const (
	uart0Base      = uintptr(0x60000000)
	uartFifoOffset = uintptr(0x00)
	uartStatus     = uintptr(0x1C)
	uartTxFifoMask = uint32(0xFF0000)
)

var bootBanner = "windmills: phase0 boot\r\n"

func main() {
	kmain()
}

//export kmain
func kmain() {
	uartWriteString(bootBanner)
	uartWriteString("windmills: phase1 init\r\n")
	phase1Init(kernelThread0)
	schedulerRun()
	uartWriteString("windmills: entering halt loop\r\n")
	halt()
}

func kernelThread0() {
	uartWriteString("windmills: kthread0 ran\r\n")
}

func halt() {
	for {
	}
}

func uartWriteString(s string) {
	for i := 0; i < len(s); i++ {
		uartWriteByte(s[i])
	}
}

func uartWriteByte(b byte) {
	for {
		status := *(*uint32)(unsafe.Pointer(uart0Base + uartStatus))
		if ((status & uartTxFifoMask) >> 16) < 127 {
			break
		}
	}
	*(*uint32)(unsafe.Pointer(uart0Base + uartFifoOffset)) = uint32(b)
}
