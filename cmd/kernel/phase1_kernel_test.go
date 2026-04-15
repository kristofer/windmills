package main

import "testing"

func resetPhase1ForTest() {
	interruptsEnabled = true
	interruptDisableN = 0
	tickLock = spinlock{}
	monotonicTickCount = 0
	timerConfigured = false
	timerIntervalTicks = 0
	timerIRQEnabled = false
	bootThread = kernelThread{}
}

func TestSpinlockDisablesAndRestoresInterrupts(t *testing.T) {
	resetPhase1ForTest()

	var l spinlock
	state := l.lock()
	if interruptsEnabled {
		t.Fatalf("interrupts should be disabled while lock is held")
	}
	l.unlock(state)
	if !interruptsEnabled {
		t.Fatalf("interrupts should be restored after unlock")
	}
}

func TestTimerInterruptIncrementsMonotonicTick(t *testing.T) {
	resetPhase1ForTest()
	timerInit()
	if !timerConfigured || !timerIRQEnabled || timerIntervalTicks != 1 {
		t.Fatalf("timer setup not initialized correctly")
	}

	timerInterrupt()
	timerInterrupt()
	timerInterrupt()

	if got, want := monotonicTick(), uint64(3); got != want {
		t.Fatalf("monotonicTick() = %d, want %d", got, want)
	}
}

func TestSchedulerRunsSingleKernelThread(t *testing.T) {
	resetPhase1ForTest()
	ran := 0

	schedulerInit(func() {
		ran++
	})

	schedulerRun()
	schedulerRun()

	if ran != 1 {
		t.Fatalf("kernel thread ran %d times, want 1", ran)
	}
	if bootThread.state != threadExited {
		t.Fatalf("boot thread state = %v, want %v", bootThread.state, threadExited)
	}
}
