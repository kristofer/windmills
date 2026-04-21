//go:build tinygo && esp32s3

package main

import (
	"time"
)

func main() {
	// Give the USB CDC host time to re-enumerate after the hard reset
	// performed by esptool.
	time.Sleep(time.Millisecond * 4000)

	consoleLogln("windmills: phase0 boot")

	phase1Init(kernelThread0)
	consoleLogln("windmills: phase1 init")

	phase2Init()
	consoleLogln("windmills: phase2 init heap ready")

	schedulerRun()

	consoleLogln("windmills: entering halt loop")

	// Extra sleep so USB CDC has settled before the first dump attempt.
	time.Sleep(time.Millisecond * 3000)

	// Retry dump + alive every second. dumpConsoleLog resets the buffer after
	// success, so boot messages appear exactly once once USB is ready. A
	// late-connecting monitor always sees the alive heartbeat.
	for {
		dumpConsoleLog()
		consoleWriteString("windmills: alive\n")
		busyDelay(1000)
	}
}

func kernelThread0() {
	consoleLogln("windmills: kthread0 ran")
}

func halt() {
	for {
	}
}

