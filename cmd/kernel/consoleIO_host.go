//go:build !tinygo

package main

import "time"

func consoleWriteString(_ string) {}

func consoleLogln(_ string) {}

func dumpConsoleLog() {}

func busyDelay(ms time.Duration) {
	time.Sleep(time.Millisecond * ms)
}
