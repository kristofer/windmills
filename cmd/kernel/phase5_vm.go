package main

import "unsafe"

type vmPerm uint8

const (
	vmPermRead vmPerm = 1 << iota
	vmPermWrite
	vmPermExec
	vmPermUser
)

const (
	maxVMPageMappings = 128

	userTextStart  = uintptr(0x00100000)
	userDataStart  = uintptr(0x00200000)
	userStackTop   = uintptr(0x7FFF0000)
	userTrapPageVA = uintptr(0x7FFFF000)
	userStackPages = uintptr(16)
	vmNoGuard      = uintptr(0)
)

type vmPageMapping struct {
	vaddr   uintptr
	paddr   uintptr
	perm    vmPerm
	pool    allocatorPool
	present bool
	owned   bool
}

type vmPageTable struct {
	mappings [maxVMPageMappings]vmPageMapping
}

type vmLayout struct {
	textStart  uintptr
	dataStart  uintptr
	stackTop   uintptr
	stackBase  uintptr
	stackFloor uintptr
	guardStart uintptr
	guardEnd   uintptr
	trapPage   uintptr
}

type vmAddressSpace struct {
	layout vmLayout
	ptable vmPageTable
}

func pageAlignDown(address uintptr) uintptr {
	return address &^ (pageSizeBytes - 1)
}

func vmFindMapping(pt *vmPageTable, vaddr uintptr) (*vmPageMapping, bool) {
	page := pageAlignDown(vaddr)
	for i := range pt.mappings {
		m := &pt.mappings[i]
		if !m.present || m.vaddr != page {
			continue
		}
		return m, true
	}
	return nil, false
}

func vmMapPage(pt *vmPageTable, vaddr, paddr uintptr, perm vmPerm) bool {
	if vaddr%pageSizeBytes != 0 || paddr%pageSizeBytes != 0 {
		return false
	}
	if entry, ok := vmFindMapping(pt, vaddr); ok {
		entry.paddr = paddr
		entry.perm = perm
		entry.present = true
		entry.owned = false
		entry.pool = kernelMemoryPool
		return true
	}
	for i := range pt.mappings {
		if pt.mappings[i].present {
			continue
		}
		pt.mappings[i] = vmPageMapping{
			vaddr:   vaddr,
			paddr:   paddr,
			perm:    perm,
			present: true,
		}
		return true
	}
	return false
}

func vmMapOwnedPage(pt *vmPageTable, vaddr, paddr uintptr, pool allocatorPool, perm vmPerm) bool {
	if !vmMapPage(pt, vaddr, paddr, perm) {
		return false
	}
	entry, ok := vmFindMapping(pt, vaddr)
	if !ok {
		return false
	}
	entry.owned = true
	entry.pool = pool
	return true
}

func vmUnmapPage(pt *vmPageTable, vaddr uintptr) bool {
	entry, ok := vmFindMapping(pt, vaddr)
	if !ok {
		return false
	}
	*entry = vmPageMapping{}
	return true
}

func vmTranslate(pt *vmPageTable, vaddr uintptr, required vmPerm, requireUser bool) (uintptr, bool) {
	entry, ok := vmFindMapping(pt, vaddr)
	if !ok {
		return 0, false
	}
	if entry.perm&required != required {
		return 0, false
	}
	if requireUser && entry.perm&vmPermUser == 0 {
		return 0, false
	}
	return entry.paddr + (vaddr - entry.vaddr), true
}

func vmInitAddressSpace(as *vmAddressSpace) {
	as.layout = vmLayout{
		textStart:  userTextStart,
		dataStart:  userDataStart,
		stackTop:   userStackTop,
		stackBase:  userStackTop,
		stackFloor: userStackTop - userStackPages*pageSizeBytes,
		trapPage:   userTrapPageVA,
	}
	as.layout.guardStart = vmNoGuard
	as.layout.guardEnd = vmNoGuard
}

func vmRefreshGuard(as *vmAddressSpace) {
	if as.layout.stackBase == as.layout.stackTop {
		as.layout.guardStart = vmNoGuard
		as.layout.guardEnd = vmNoGuard
		return
	}
	as.layout.guardStart = as.layout.stackBase - pageSizeBytes
	as.layout.guardEnd = as.layout.stackBase
}

func vmInitProcess(p *process) bool {
	vmInitAddressSpace(&p.vm)
	trapPage, ok := allocPageFromPool(kernelMemoryPool)
	if !ok {
		return false
	}
	if !vmMapOwnedPage(&p.vm.ptable, p.vm.layout.trapPage, trapPage, kernelMemoryPool, vmPermRead) {
		if !freePageInPool(trapPage, kernelMemoryPool) {
			panic("vm: failed to free trap page")
		}
		return false
	}
	return true
}

func vmReleaseProcess(p *process) {
	for i := range p.vm.ptable.mappings {
		entry := &p.vm.ptable.mappings[i]
		if !entry.present || !entry.owned {
			continue
		}
		if !freePageInPool(entry.paddr, entry.pool) {
			panic("vm: failed to free mapped page")
		}
		p.vm.ptable.mappings[i] = vmPageMapping{}
	}
	p.vm = vmAddressSpace{}
}

func vmGrowStack(p *process, faultVA uintptr) bool {
	page := pageAlignDown(faultVA)
	if page >= p.vm.layout.stackTop || page < p.vm.layout.stackFloor {
		return false
	}
	if page+pageSizeBytes != p.vm.layout.stackBase {
		return false
	}
	phys, ok := allocPageFromPool(userMemoryPool)
	if !ok {
		return false
	}
	if !vmMapOwnedPage(&p.vm.ptable, page, phys, userMemoryPool, vmPermRead|vmPermWrite|vmPermUser) {
		if !freePageInPool(phys, userMemoryPool) {
			panic("vm: failed to free stack page")
		}
		return false
	}
	p.vm.layout.stackBase = page
	vmRefreshGuard(&p.vm)
	return true
}

func vmHandleFault(p *process, faultVA uintptr, write bool) bool {
	required := vmPermRead
	if write {
		required |= vmPermWrite
	}
	if _, ok := vmTranslate(&p.vm.ptable, faultVA, required, true); ok {
		return true
	}
	if p.vm.layout.guardStart != vmNoGuard && faultVA >= p.vm.layout.guardStart && faultVA < p.vm.layout.guardEnd {
		p.faulted = true
		return false
	}
	if vmGrowStack(p, faultVA) {
		return true
	}
	p.faulted = true
	return false
}

func copyin(p *process, dst []byte, srcVA uintptr) bool {
	if p == nil {
		return false
	}
	for i := 0; i < len(dst); i++ {
		va := srcVA + uintptr(i)
		pa, ok := vmTranslate(&p.vm.ptable, va, vmPermRead, true)
		if !ok {
			if !vmHandleFault(p, va, false) {
				return false
			}
			pa, ok = vmTranslate(&p.vm.ptable, va, vmPermRead, true)
			if !ok {
				p.faulted = true
				return false
			}
		}
		dst[i] = *(*byte)(unsafe.Pointer(pa))
	}
	return true
}

func copyout(p *process, dstVA uintptr, src []byte) bool {
	if p == nil {
		return false
	}
	for i := 0; i < len(src); i++ {
		va := dstVA + uintptr(i)
		pa, ok := vmTranslate(&p.vm.ptable, va, vmPermWrite, true)
		if !ok {
			if !vmHandleFault(p, va, true) {
				return false
			}
			pa, ok = vmTranslate(&p.vm.ptable, va, vmPermWrite, true)
			if !ok {
				p.faulted = true
				return false
			}
		}
		*(*byte)(unsafe.Pointer(pa)) = src[i]
	}
	return true
}
