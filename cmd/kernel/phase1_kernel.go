package main

import "sync/atomic"

type irqState struct {
	enabled bool
}

type spinlock struct {
	locked uint32
}

type threadState uint8

const (
	threadRunnable threadState = iota
	threadRunning
	threadExited
)

type kernelThread struct {
	name  string
	entry func()
	state threadState
}

var (
	interruptsEnabled  = true
	interruptDisableN  uint32
	tickLock           spinlock
	monotonicTickCount uint64
	timerConfigured    bool
	timerIntervalTicks uint32
	timerIRQEnabled    bool
	bootThread         kernelThread
)

func irqDisableSave() irqState {
	state := irqState{enabled: interruptsEnabled}
	interruptsEnabled = false
	interruptDisableN++
	return state
}

func irqRestore(state irqState) {
	if interruptDisableN == 0 {
		return
	}
	interruptDisableN--
	if interruptDisableN == 0 && state.enabled {
		interruptsEnabled = true
	}
}

func (l *spinlock) lock() irqState {
	state := irqDisableSave()
	if !atomic.CompareAndSwapUint32(&l.locked, 0, 1) {
		panic("spinlock: already locked")
	}
	return state
}

func (l *spinlock) unlock(state irqState) {
	if !atomic.CompareAndSwapUint32(&l.locked, 1, 0) {
		panic("spinlock: not locked")
	}
	irqRestore(state)
}

func timerInit() {
	state := tickLock.lock()
	timerConfigured = true
	timerIntervalTicks = 1
	timerIRQEnabled = true
	tickLock.unlock(state)
}

func timerInterrupt() {
	state := tickLock.lock()
	if !timerConfigured || !timerIRQEnabled {
		tickLock.unlock(state)
		return
	}
	monotonicTickCount++
	tick := monotonicTickCount
	tickLock.unlock(state)
	schedulerOnTick(tick)
}

func monotonicTick() uint64 {
	state := tickLock.lock()
	tick := monotonicTickCount
	tickLock.unlock(state)
	return tick
}

func schedulerInit(entry func()) {
	if entry == nil {
		panic("scheduler: nil entry")
	}
	bootThread = kernelThread{
		name:  "kthread0",
		entry: entry,
		state: threadRunnable,
	}
}

func schedulerRun() {
	if bootThread.state != threadRunnable {
		return
	}
	bootThread.state = threadRunning
	bootThread.entry()
	if bootThread.state == threadRunning {
		bootThread.state = threadExited
	}
}

func schedulerYield() {
	// Cooperative scheduler entrypoint for future trap/syscall-driven yields.
	if bootThread.state == threadRunning {
		bootThread.state = threadRunnable
	}
}

func schedulerOnTick(tick uint64) {
	_ = tick
	// Cooperative scheduler skeleton: timer currently tracks monotonic time only.
}

func phase1Init(entry func()) {
	timerInit()
	schedulerInit(entry)
}
