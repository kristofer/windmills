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
	for i := range processTable {
		processTable[i] = process{
			slot:  i,
			state: procUnused,
		}
	}
	nextPID = 1
	currentProc = nil
	bootstrap = nil
	lastScheduledSlot = -1
	schedulerReady = false
	runDepth = 0
	scheduleRequested = false
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

func TestSchedulerRunsBootstrapProcess(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase2Init()
	ran := 0

	schedulerInit(func() {
		ran++
	})

	schedulerRun()
	schedulerRun()

	if ran != 1 {
		t.Fatalf("bootstrap process ran %d times, want 1", ran)
	}
	initProc := findProcByPID(1)
	if initProc == nil {
		t.Fatalf("init process should exist")
	}
	if initProc.state != procZombie {
		t.Fatalf("init process state = %v, want %v", initProc.state, procZombie)
	}
}

func TestProcessTableHonorsNPROC(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase2Init()
	schedulerInit(func() {})

	for i := 0; i < NPROC; i++ {
		if _, ok := processCreateKernel("worker", func() {}, 0); !ok {
			t.Fatalf("expected process allocation %d to succeed", i)
		}
	}

	if _, ok := processCreateKernel("overflow", func() {}, 0); ok {
		t.Fatalf("expected allocation beyond NPROC to fail")
	}
}

func TestTimerInterruptPreemptsCurrentProcessRoundRobin(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase2Init()

	sequence := make([]int, 0, 3)
	firstRun := true

	schedulerInit(func() {
		if firstRun {
			firstRun = false
			sequence = append(sequence, 1)
			timerInterrupt()
			sequence = append(sequence, 99)
			return
		}
		sequence = append(sequence, 3)
	})
	timerInit()

	if !ensureBootstrapProcess() {
		t.Fatalf("expected bootstrap process to be created")
	}
	if _, ok := processCreateKernel("worker", func() {
		sequence = append(sequence, 2)
	}, 0); !ok {
		t.Fatalf("expected worker process allocation")
	}

	schedulerRun()

	if len(sequence) != 3 {
		t.Fatalf("sequence length = %d, want 3 (%v)", len(sequence), sequence)
	}
	if sequence[0] != 1 || sequence[1] != 2 || sequence[2] != 3 {
		t.Fatalf("unexpected schedule sequence: %v", sequence)
	}
}

func TestTrapDispatchForkWaitExitGetpid(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase2Init()

	stage := 0
	var parentPID int
	var childPID int
	var waitedPID int

	schedulerInit(func() {
		getpidFrame := trapframe{Syscall: syscallGetpid}
		trapDispatch(&getpidFrame)
		pid := int(getpidFrame.ReturnValue)
		if pid == 0 {
			t.Fatalf("getpid should return non-zero pid")
		}
		if pid != 1 {
			exitChild := trapframe{Syscall: syscallExit}
			trapDispatch(&exitChild)
			return
		}

		parentPID = pid
		if stage == 0 {
			stage = 1
			forkFrame := trapframe{Syscall: syscallFork}
			trapDispatch(&forkFrame)
			childPID = int(forkFrame.ReturnValue)
			yieldFrame := trapframe{Syscall: syscallYield}
			trapDispatch(&yieldFrame)
			return
		}

		waitFrame := trapframe{Syscall: syscallWait}
		trapDispatch(&waitFrame)
		waitedPID = int(waitFrame.ReturnValue)
		exitParent := trapframe{Syscall: syscallExit}
		trapDispatch(&exitParent)
	})

	schedulerRun()

	if parentPID != 1 {
		t.Fatalf("parent pid = %d, want 1", parentPID)
	}
	if childPID <= 1 {
		t.Fatalf("fork returned child pid = %d, want > 1", childPID)
	}
	if waitedPID != childPID {
		t.Fatalf("wait returned pid = %d, want %d", waitedPID, childPID)
	}
	if findProcByPID(childPID) != nil {
		t.Fatalf("wait should reap child process")
	}
}
