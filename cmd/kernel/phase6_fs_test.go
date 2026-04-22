package main

import "testing"

func setupPhase6TestProcess(t *testing.T) *process {
	t.Helper()
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase6ResetForTest()
	_ = configureHostBackedPhysicalMemory(t)
	phase2Init()
	if !phase6Init() {
		t.Fatalf("phase6Init should succeed")
	}
	p, ok := allocProcess("phase6", func() {}, 0)
	if !ok {
		t.Fatalf("allocProcess should succeed")
	}
	t.Cleanup(func() {
		releaseProcess(p)
		currentProc = nil
	})
	currentProc = p
	return p
}

func mapUserRWPage(t *testing.T, p *process, va uintptr) {
	t.Helper()
	page, ok := allocPageFromPool(userMemoryPool)
	if !ok {
		t.Fatalf("allocPageFromPool should succeed")
	}
	clearMemory(page, pageSizeBytes)
	if !vmMapOwnedPage(&p.vm.ptable, pageAlignDown(va), page, userMemoryPool, vmPermRead|vmPermWrite|vmPermUser) {
		t.Fatalf("vmMapOwnedPage should succeed")
	}
}

func copyCStringToUser(t *testing.T, p *process, dstVA uintptr, value string) {
	t.Helper()
	if !copyout(p, dstVA, append([]byte(value), 0)) {
		t.Fatalf("copyout path should succeed")
	}
}

func TestPhase6InitProvidesInitAndDeviceNodes(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	phase6ResetForTest()
	phase2Init()
	if !phase6Init() {
		t.Fatalf("phase6Init should succeed")
	}
	if node, ok := fsLookupByPath(nil, "/init"); !ok || node.kind != inodeKindFile {
		t.Fatalf("/init should be present as a file")
	}
	if node, ok := fsLookupByPath(nil, "/dev/console"); !ok || node.device != deviceConsole {
		t.Fatalf("/dev/console should be a console device")
	}
	if node, ok := fsLookupByPath(nil, "/dev/uart"); !ok || node.device != deviceUART {
		t.Fatalf("/dev/uart should be a UART device")
	}
	if node, ok := fsLookupByPath(nil, "/dev/timer"); !ok || node.device != deviceTimer {
		t.Fatalf("/dev/timer should be a timer device")
	}
}

func TestPhase6SyscallsOpenReadWriteCloseAndDevices(t *testing.T) {
	p := setupPhase6TestProcess(t)

	pathVA := pageAlignDown(userDataStart)
	mapUserRWPage(t, p, pathVA)
	payloadVA := pathVA + 256
	readbackVA := pathVA + 512

	copyCStringToUser(t, p, pathVA, "/tmp.txt")
	openFrame := trapframe{Syscall: syscallOpen, Args: [3]uintptr{pathVA, openRead | openWrite | openCreate}}
	trapDispatch(&openFrame)
	if openFrame.ReturnValue == syscallError {
		t.Fatalf("open create should succeed")
	}
	fd := int(openFrame.ReturnValue)

	payload := []byte("windmills-phase6")
	if !copyout(p, payloadVA, payload) {
		t.Fatalf("copyout payload should succeed")
	}
	writeFrame := trapframe{Syscall: syscallWrite, Args: [3]uintptr{uintptr(fd), payloadVA, uintptr(len(payload))}}
	trapDispatch(&writeFrame)
	if got := int(writeFrame.ReturnValue); got != len(payload) {
		t.Fatalf("write returned %d, want %d", got, len(payload))
	}

	closeFrame := trapframe{Syscall: syscallClose, Args: [3]uintptr{uintptr(fd)}}
	trapDispatch(&closeFrame)
	if closeFrame.ReturnValue == syscallError {
		t.Fatalf("close should succeed")
	}

	reopenFrame := trapframe{Syscall: syscallOpen, Args: [3]uintptr{pathVA, openRead}}
	trapDispatch(&reopenFrame)
	if reopenFrame.ReturnValue == syscallError {
		t.Fatalf("open read should succeed")
	}
	fd = int(reopenFrame.ReturnValue)

	readFrame := trapframe{Syscall: syscallRead, Args: [3]uintptr{uintptr(fd), readbackVA, uintptr(len(payload))}}
	trapDispatch(&readFrame)
	if got := int(readFrame.ReturnValue); got != len(payload) {
		t.Fatalf("read returned %d, want %d", got, len(payload))
	}
	readback := make([]byte, len(payload))
	if !copyin(p, readback, readbackVA) {
		t.Fatalf("copyin readback should succeed")
	}
	if string(readback) != string(payload) {
		t.Fatalf("readback = %q, want %q", string(readback), string(payload))
	}

	copyCStringToUser(t, p, pathVA, "/dev/console")
	openConsole := trapframe{Syscall: syscallOpen, Args: [3]uintptr{pathVA, openWrite}}
	trapDispatch(&openConsole)
	if openConsole.ReturnValue == syscallError {
		t.Fatalf("open /dev/console should succeed")
	}
	consoleFD := int(openConsole.ReturnValue)
	consoleMsg := []byte("hello-console")
	if !copyout(p, payloadVA, consoleMsg) {
		t.Fatalf("copyout console payload should succeed")
	}
	writeConsole := trapframe{Syscall: syscallWrite, Args: [3]uintptr{uintptr(consoleFD), payloadVA, uintptr(len(consoleMsg))}}
	trapDispatch(&writeConsole)
	if got := int(writeConsole.ReturnValue); got != len(consoleMsg) {
		t.Fatalf("console write returned %d, want %d", got, len(consoleMsg))
	}
	if got := string(consoleDeviceSink); got != string(consoleMsg) {
		t.Fatalf("console sink = %q, want %q", got, string(consoleMsg))
	}
}

func TestPhase6SyscallsMkdirChdirLinkUnlink(t *testing.T) {
	p := setupPhase6TestProcess(t)
	pathVA := pageAlignDown(userDataStart)
	mapUserRWPage(t, p, pathVA)
	dataVA := pathVA + 256

	copyCStringToUser(t, p, pathVA, "/tmp")
	mkdirFrame := trapframe{Syscall: syscallMkdir, Args: [3]uintptr{pathVA}}
	trapDispatch(&mkdirFrame)
	if mkdirFrame.ReturnValue == syscallError {
		t.Fatalf("mkdir should succeed")
	}

	chdirFrame := trapframe{Syscall: syscallChdir, Args: [3]uintptr{pathVA}}
	trapDispatch(&chdirFrame)
	if chdirFrame.ReturnValue == syscallError {
		t.Fatalf("chdir should succeed")
	}

	copyCStringToUser(t, p, pathVA, "note")
	openFrame := trapframe{Syscall: syscallOpen, Args: [3]uintptr{pathVA, openRead | openWrite | openCreate}}
	trapDispatch(&openFrame)
	if openFrame.ReturnValue == syscallError {
		t.Fatalf("open relative note should succeed")
	}
	fd := int(openFrame.ReturnValue)
	content := []byte("linked-content")
	if !copyout(p, dataVA, content) {
		t.Fatalf("copyout content should succeed")
	}
	writeFrame := trapframe{Syscall: syscallWrite, Args: [3]uintptr{uintptr(fd), dataVA, uintptr(len(content))}}
	trapDispatch(&writeFrame)
	if writeFrame.ReturnValue == syscallError {
		t.Fatalf("write should succeed")
	}

	newPathVA := pathVA + 128
	copyCStringToUser(t, p, pathVA, "note")
	copyCStringToUser(t, p, newPathVA, "note2")
	linkFrame := trapframe{Syscall: syscallLink, Args: [3]uintptr{pathVA, newPathVA}}
	trapDispatch(&linkFrame)
	if linkFrame.ReturnValue == syscallError {
		t.Fatalf("link should succeed")
	}

	unlinkOld := trapframe{Syscall: syscallUnlink, Args: [3]uintptr{pathVA}}
	trapDispatch(&unlinkOld)
	if unlinkOld.ReturnValue == syscallError {
		t.Fatalf("unlink old name should succeed")
	}

	reopenNote := trapframe{Syscall: syscallOpen, Args: [3]uintptr{pathVA, openRead}}
	trapDispatch(&reopenNote)
	if reopenNote.ReturnValue != syscallError {
		t.Fatalf("old path should no longer exist")
	}

	openLinked := trapframe{Syscall: syscallOpen, Args: [3]uintptr{newPathVA, openRead}}
	trapDispatch(&openLinked)
	if openLinked.ReturnValue == syscallError {
		t.Fatalf("linked path should remain readable")
	}
}
