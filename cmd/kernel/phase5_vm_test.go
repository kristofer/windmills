package main

import "testing"

const (
	testStackOffsetFromTop   = uintptr(8)
	testGuardOffsetFromStart = uintptr(1)
)

func TestPhase5ProcessLayoutIncludesStackGuardAndTrapPage(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	_ = configureHostBackedPhysicalMemory(t)
	phase2Init()

	p, ok := allocProcess("vmtest", func() {}, 0)
	if !ok {
		t.Fatalf("expected process allocation to succeed")
	}
	defer releaseProcess(p)

	if p.vm.layout.textStart != userTextStart {
		t.Fatalf("textStart = 0x%x, want 0x%x", p.vm.layout.textStart, userTextStart)
	}
	if p.vm.layout.dataStart != userDataStart {
		t.Fatalf("dataStart = 0x%x, want 0x%x", p.vm.layout.dataStart, userDataStart)
	}
	if p.vm.layout.stackTop != userStackTop {
		t.Fatalf("stackTop = 0x%x, want 0x%x", p.vm.layout.stackTop, userStackTop)
	}
	if p.vm.layout.stackBase != userStackTop {
		t.Fatalf("stackBase = 0x%x, want 0x%x", p.vm.layout.stackBase, userStackTop)
	}
	if p.vm.layout.trapPage != userTrapPageVA {
		t.Fatalf("trapPage = 0x%x, want 0x%x", p.vm.layout.trapPage, userTrapPageVA)
	}
	if _, ok := vmTranslate(&p.vm.ptable, p.vm.layout.trapPage, vmPermRead, true); ok {
		t.Fatalf("trap page must not be user-accessible")
	}
}

func TestPhase5CopyinCopyoutHonorUserPermissions(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	_ = configureHostBackedPhysicalMemory(t)
	phase2Init()

	p, ok := allocProcess("vmtest", func() {}, 0)
	if !ok {
		t.Fatalf("expected process allocation to succeed")
	}
	defer releaseProcess(p)

	userPage, ok := allocPageFromPool(userMemoryPool)
	if !ok {
		t.Fatalf("expected user page allocation")
	}
	clearMemory(userPage, pageSizeBytes)
	dataVA := pageAlignDown(userDataStart)
	if !vmMapOwnedPage(&p.vm.ptable, dataVA, userPage, userMemoryPool, vmPermRead|vmPermWrite|vmPermUser) {
		t.Fatalf("expected user mapping")
	}

	payload := []byte("windmills")
	if !copyout(p, dataVA, payload) {
		t.Fatalf("copyout should succeed to user page")
	}
	readback := make([]byte, len(payload))
	if !copyin(p, readback, dataVA) {
		t.Fatalf("copyin should succeed from user page")
	}
	if string(readback) != string(payload) {
		t.Fatalf("readback = %q, want %q", string(readback), string(payload))
	}

	kernelPage, ok := allocPageFromPool(kernelMemoryPool)
	if !ok {
		t.Fatalf("expected kernel page allocation")
	}
	clearMemory(kernelPage, pageSizeBytes)
	kernelOnlyVA := pageAlignDown(userDataStart + pageSizeBytes)
	if !vmMapOwnedPage(&p.vm.ptable, kernelOnlyVA, kernelPage, kernelMemoryPool, vmPermRead|vmPermWrite) {
		t.Fatalf("expected kernel mapping")
	}
	if copyout(p, kernelOnlyVA, []byte{1}) {
		t.Fatalf("copyout should reject kernel-only mapping")
	}
	if copyin(p, make([]byte, 1), kernelOnlyVA) {
		t.Fatalf("copyin should reject kernel-only mapping")
	}
}

func TestPhase5LazyStackGrowthAndFaultHandling(t *testing.T) {
	resetPhase1ForTest()
	resetPhase2ForTest()
	_ = configureHostBackedPhysicalMemory(t)
	phase2Init()

	p, ok := allocProcess("vmtest", func() {}, 0)
	if !ok {
		t.Fatalf("expected process allocation to succeed")
	}
	defer releaseProcess(p)

	stackAddr := p.vm.layout.stackTop - testStackOffsetFromTop
	if !copyout(p, stackAddr, []byte{0xAB}) {
		t.Fatalf("copyout should trigger lazy stack growth")
	}
	if p.vm.layout.stackBase != p.vm.layout.stackTop-pageSizeBytes {
		t.Fatalf("stackBase = 0x%x, want 0x%x", p.vm.layout.stackBase, p.vm.layout.stackTop-pageSizeBytes)
	}

	guardAddr := p.vm.layout.guardStart + testGuardOffsetFromStart
	if guardAddr < p.vm.layout.guardEnd && copyout(p, guardAddr, []byte{0xCD}) {
		t.Fatalf("copyout into guard page should fail")
	}
	if !p.faulted {
		t.Fatalf("guard page fault should mark process faulted")
	}
}
