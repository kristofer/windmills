package main

import (
	"sync/atomic"
	"time"
)

type irqState struct {
	enabled bool
}

type spinlock struct {
	locked uint32
}

const NPROC = 16

type procState uint8

const (
	procUnused procState = iota
	procEmbryo
	procRunnable
	procRunning
	procSleeping
	procZombie
)

type trapframe struct {
	Syscall     uint32
	Args        [3]uintptr
	ReturnValue uintptr
}

type savedContext struct {
	sp uintptr
	pc uintptr
}

type process struct {
	slot             int
	pid              int
	parentPID        int
	name             string
	state            procState
	entry            func()
	kernelStackBase  uintptr
	kernelStackPages uintptr
	trapframe        trapframe
	context          savedContext
	exitStatus       int
}

const (
	syscallFork uint32 = iota + 1
	syscallExit
	syscallWait
	syscallYield
	syscallGetpid
)

var (
	interruptsEnabled  = true
	interruptDisableN  uint32
	tickLock           spinlock
	monotonicTickCount uint64
	timerConfigured    bool
	timerIntervalTicks uint32
	timerIRQEnabled    bool

	processTable [NPROC]process
	nextPID      int
	currentProc  *process
	bootstrap    func()

	lastScheduledSlot int
	schedulerReady    bool
	runDepth          int
	scheduleRequested bool
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
	if interruptDisableN > 1 && atomic.LoadUint32(&l.locked) != 0 {
		panic("spinlock: recursive lock")
	}
	for !atomic.CompareAndSwapUint32(&l.locked, 0, 1) {
		time.Sleep(time.Microsecond)
	}
	return state
}

func (l *spinlock) unlock(state irqState) {
	if atomic.LoadUint32(&l.locked) == 0 {
		panic("spinlock: not locked")
	}
	atomic.StoreUint32(&l.locked, 0)
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
	tickLock.unlock(state)
	schedulerOnTick()
	if scheduleRequested && runDepth > 0 {
		panic(preemptSignal{})
	}
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
	bootstrap = entry
	for i := range processTable {
		processTable[i] = process{
			slot:  i,
			state: procUnused,
		}
	}
	nextPID = 1
	currentProc = nil
	lastScheduledSlot = -1
	scheduleRequested = false
	schedulerReady = true
}

func allocProcess(name string, entry func(), parentPID int) (*process, bool) {
	if entry == nil {
		return nil, false
	}
	kstack, ok := AllocContig(1)
	if !ok {
		return nil, false
	}
	for i := range processTable {
		if processTable[i].state != procUnused {
			continue
		}
		p := &processTable[i]
		*p = process{
			slot:             i,
			pid:              nextPID,
			parentPID:        parentPID,
			name:             name,
			state:            procEmbryo,
			entry:            entry,
			kernelStackBase:  kstack,
			kernelStackPages: 1,
		}
		p.context.sp = kstack + pageSizeBytes
		nextPID++
		p.state = procRunnable
		return p, true
	}
	_ = FreePage(kstack)
	return nil, false
}

func processCreateKernel(name string, entry func(), parentPID int) (int, bool) {
	p, ok := allocProcess(name, entry, parentPID)
	if !ok {
		return 0, false
	}
	return p.pid, true
}

func findProcByPID(pid int) *process {
	for i := range processTable {
		if processTable[i].state == procUnused || processTable[i].pid != pid {
			continue
		}
		return &processTable[i]
	}
	return nil
}

func ensureBootstrapProcess() bool {
	if findProcByPID(1) != nil {
		return true
	}
	if bootstrap == nil {
		return false
	}
	_, ok := allocProcess("init", bootstrap, 0)
	return ok
}

func pickNextRunnable() *process {
	for offset := 1; offset <= NPROC; offset++ {
		slot := (lastScheduledSlot + offset) % NPROC
		if processTable[slot].state != procRunnable {
			continue
		}
		lastScheduledSlot = slot
		return &processTable[slot]
	}
	return nil
}

type yieldSignal struct{}
type preemptSignal struct{}
type exitSignal struct {
	status int
}

func switchToProcess(p *process) {
	p.state = procRunning
	currentProc = p
	scheduleRequested = false
	runDepth++
	defer func() {
		runDepth--
		if recovered := recover(); recovered != nil {
			switch signal := recovered.(type) {
			case yieldSignal:
				if p.state == procRunning {
					p.state = procRunnable
				}
			case preemptSignal:
				if p.state == procRunning {
					p.state = procRunnable
				}
			case exitSignal:
				processExit(p, signal.status)
			default:
				panic(recovered)
			}
		}
		if p.state == procRunning {
			processExit(p, 0)
		}
		if currentProc == p {
			currentProc = nil
		}
	}()
	p.entry()
}

func schedulerRun() {
	if !schedulerReady || !mem_init_complete {
		return
	}
	if !ensureBootstrapProcess() {
		return
	}
	for {
		next := pickNextRunnable()
		if next == nil {
			return
		}
		switchToProcess(next)
	}
}

func schedulerYield() {
	if currentProc == nil {
		return
	}
	if currentProc.state == procRunning {
		currentProc.state = procRunnable
	}
	scheduleRequested = true
	panic(yieldSignal{})
}

func schedulerOnTick() {
	if currentProc == nil || currentProc.state != procRunning {
		return
	}
	currentProc.state = procRunnable
	scheduleRequested = true
}

func processExit(p *process, status int) {
	p.exitStatus = status
	p.state = procZombie
	if p.parentPID != 0 {
		parent := findProcByPID(p.parentPID)
		if parent != nil && parent.state == procSleeping {
			parent.state = procRunnable
		}
	}
}

func releaseProcess(p *process) {
	for page := uintptr(0); page < p.kernelStackPages; page++ {
		_ = FreePage(p.kernelStackBase + page*pageSizeBytes)
	}
	slot := p.slot
	*p = process{
		slot:  slot,
		state: procUnused,
	}
}

func syscallDispatch(p *process, tf *trapframe) uintptr {
	switch tf.Syscall {
	case syscallFork:
		child, ok := allocProcess(p.name, p.entry, p.pid)
		if !ok {
			return ^uintptr(0)
		}
		child.trapframe = p.trapframe
		child.context = p.context
		child.trapframe.ReturnValue = 0
		return uintptr(child.pid)
	case syscallExit:
		panic(exitSignal{status: int(tf.Args[0])})
	case syscallWait:
		hasChildren := false
		for i := range processTable {
			child := &processTable[i]
			if child.state == procUnused || child.parentPID != p.pid {
				continue
			}
			hasChildren = true
			if child.state == procZombie {
				pid := child.pid
				releaseProcess(child)
				return uintptr(pid)
			}
		}
		if !hasChildren {
			return ^uintptr(0)
		}
		return 0
	case syscallYield:
		schedulerYield()
		return 0
	case syscallGetpid:
		return uintptr(p.pid)
	default:
		return ^uintptr(0)
	}
}

func trapDispatch(tf *trapframe) {
	if tf == nil {
		return
	}
	if currentProc == nil {
		tf.ReturnValue = ^uintptr(0)
		return
	}
	currentProc.trapframe = *tf
	tf.ReturnValue = syscallDispatch(currentProc, tf)
	currentProc.trapframe = *tf
}

func phase1Init(entry func()) {
	timerInit()
	schedulerInit(entry)
}
