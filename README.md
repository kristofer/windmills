# windmills

`windmills` is a TinyGo operating system for ESP32-S3 inspired by xv6.

This repository currently defines the high-level implementation plan and system
specification that starts from a minimal bootable kernel and grows in phases
until the system is functionally comparable to xv6. [Code for xv6](https://github.com/mit-pdos/xv6-riscv)

## Phase 0 baseline (implemented)

- TinyGo target config: `targets/esp32s3-windmills.json`
- Linker layout: `targets/esp32s3-windmills.ld`
- Startup stubs: `cmd/kernel/startup_xtensa.S`
- Kernel bring-up entrypoint and UART diagnostics: `cmd/kernel/main_tinygo.go`
- UART MMIO constants use ESP32-S3 TRM `UART0` base mapping (`0x60000000`)
- Deterministic build entrypoint: `make firmware` (uses `SOURCE_DATE_EPOCH`, `TZ=UTC`, `LC_ALL=C`)

## Phase 1 skeleton (implemented)

- Xtensa interrupt/trap skeleton symbols in `cmd/kernel/startup_xtensa.S`:
  `windmills_vector_table`, `windmills_trap_entry`, `windmills_trap_exit`
- Timer skeleton and monotonic tick counter: `cmd/kernel/phase1_kernel.go`
- Interrupt disable/restore primitives and basic spinlock: `cmd/kernel/phase1_kernel.go`
- Cooperative scheduler skeleton with one kernel thread (`kthread0`):
  `cmd/kernel/main_tinygo.go`, `cmd/kernel/phase1_kernel.go`

### Phase 1 test strategy

Host tests (`go test ./...`) validate the core skeleton behavior:

- spinlock lock/unlock disables/restores interrupt state
- timer interrupt handler monotonically increments tick count
- scheduler runs the single kernel thread once

Hardware bring-up and USB flashing steps are documented in `develop.md`, including
`make flash` / `make monitor` usage and a current code-review summary.

## Phase 2 physical memory ownership (implemented)

- Physical memory map with reserved ROM/IRAM/peripheral ranges and DRAM ownership:
  `cmd/kernel/phase2_memory.go`
- 4 KiB page-frame allocator over an 8 MiB DRAM window with configurable base
  address: `cmd/kernel/phase2_memory.go`
- Strict early boot allocator reserved to early init and permanently disabled
  after memory ownership setup: `cmd/kernel/phase2_memory.go`
- Safety invariants in allocator paths to prevent returning reserved regions:
  `cmd/kernel/phase2_memory.go`

### Phase 2 test strategy

Host tests (`go test ./...`) validate:

- memory map initialization and phase2 boot allocator disable gate
- boot allocator permanent-disable behavior
- page allocator never returns reserved regions and honors configurable DRAM base

## Design constraints

- Language: TinyGo + small amounts of Xtensa assembly (no C).
- CPU target: ESP32-S3, dual-core Xtensa LX7.
- Memory architecture: Harvard (separate instruction/data memory regions).
- Early boot rule: no heap/non-stack allocation until the kernel controls
  physical memory and provides a runtime allocator.

## xv6 parity roadmap (phased plan)

### Phase 0 - Toolchain + bring-up baseline

Goal: produce deterministic firmware images and boot on ESP32-S3.

- Set up TinyGo target, linker layout, startup assembly stubs, and serial logs.
- Boot to `kmain()` on one core, print panic-safe diagnostics, halt cleanly.
- Keep kernel state static/global only (no dynamic allocation).

### Phase 1 - Single-core kernel skeleton

Goal: establish the minimum kernel execution environment.

- Interrupt vector table and trap entry/exit in Xtensa assembly.
- Timer interrupt setup and monotonic tick counter.
- Basic spinlocks and interrupt disable/restore primitives.
- Cooperative scheduler skeleton with one kernel thread.

### Phase 2 - Physical memory ownership

Goal: kernel owns all usable RAM and can allocate pages safely.

- Build memory map (reserved ROM/IRAM/DRAM/peripherals).
- Implement page-frame allocator (free-list/bitmap over 4KiB frames).
- Strict boot allocator for early init only, then permanently disabled.
- Add invariants: allocator never returns reserved regions.

### Phase 3 - TinyGo runtime allocator integration

Goal: allow Go heap use only after kernel memory control is established.

- Define kernel allocator interface: `AllocPage`, `FreePage`, `AllocContig`.
- Add TinyGo runtime hooks to request memory from kernel-managed frames.
- Keep kernel/user memory pools separate from the first runtime integration.
- Enable guarded heap allocation only after `mem_init_complete=true`.

### Phase 4 - Processes, context switching, and syscalls (implemented)

Goal: move from kernel-thread model to xv6-like process model.

- Process table with fixed upper bound (`NPROC`) and explicit states.
- Per-process kernel stack, trapframe, and saved register context.
- Round-robin preemptive scheduler using timer interrupts.
- Syscall ABI + trap dispatch (`fork`, `exit`, `wait`, `yield`, `getpid` first).

## Phase 4 process model (implemented)

- Fixed-size process table (`NPROC = 16`) with explicit lifecycle states
  (`UNUSED`, `EMBRYO`, `RUNNABLE`, `RUNNING`, `SLEEPING`, `ZOMBIE`):
  `cmd/kernel/phase1_kernel.go`
- Per-process kernel stack allocated from the page allocator,
  `trapframe`, and `savedContext` metadata:
  `cmd/kernel/phase1_kernel.go`
- Round-robin preemptive scheduler: `schedulerRun`, `pickNextRunnable`,
  `switchToProcess` driven by timer-interrupt preemption signals:
  `cmd/kernel/phase1_kernel.go`
- Syscall trap dispatch (`trapDispatch`) with initial syscall set:
  `fork`, `exit`, `wait`, `yield`, `getpid`:
  `cmd/kernel/phase1_kernel.go`

### Phase 4 test strategy

Host tests (`go test ./...`) validate:

- bootstrap process is created and transitions to `ZOMBIE` after running
- process table respects `NPROC` upper bound
- timer interrupt triggers round-robin preemption across multiple processes
- trap dispatch correctly executes `fork`, `wait`, `exit`, `yield`, and `getpid` syscall lifecycle

## Phase 5 virtual memory and address spaces (implemented)

Goal: xv6-like process isolation adapted for Xtensa/ESP32 constraints.

- Define per-process virtual layout (text/data/stack/guard/trap page concepts).
- Implement page table abstractions and mapping APIs in TinyGo.
- Kernel/user permission checks on copyin/copyout.
- Lazy user stack growth and robust fault handling.

### Phase 5 test strategy

Host tests (`go test ./...`) validate:

- per-process virtual layout initialization including stack/trap-page invariants
- page-table mapping + translation permission checks for user/kernel visibility
- `copyin`/`copyout` user-access checks across mapped and kernel-only pages
- lazy stack growth on user faults and guard-page fault rejection paths

### Phase 6 - Files, devices, and namespace

Goal: minimal xv6-like VFS and device access.

- Inode/file descriptor abstractions.
- Console, UART, and timer device files.
- Simple on-flash filesystem and init process startup (`/init` equivalent).
- Core syscalls: `open/read/write/close`, `link/unlink`, `mkdir/chdir`.

### Phase 7 - Shell + userland

Goal: establish xv6-like user workflow.

- Tiny userspace ABI and linker conventions.
- Init + shell with pipelines/redirection subset.
- Basic user utilities (`ls`, `cat`, `echo`, `sh`, `kill`, `ps`-like tool).

### Phase 8 - SMP support (second core)

Goal: dual-core execution with correct synchronization.

- Core-local data structures and scheduler state.
- Inter-processor interrupts (IPI) for wakeup/TLB-style coordination.
- Locking audit (spinlocks, sleep locks, lock ordering rules).
- Scheduler balancing and CPU affinity primitives.

### Phase 9 - Reliability and parity closure

Goal: converge toward xv6 behavior and teaching-kernel clarity.

- Full syscall coverage target aligned with selected xv6 scope.
- Stress tests for process lifecycle, FS consistency, and concurrency races.
- Kernel tracing (`trace`, `procdump`) and panic diagnostics.
- Documentation parity: architecture notes + subsystem walkthroughs.

## System specification (high level)

### Boot and initialization

1. ROM/bootloader transfers control to startup assembly.
2. Startup sets stacks, zeroes BSS, installs temporary vectors.
3. `kmain()` performs phased init:
   - platform + clocks + UART
   - interrupts/timer
   - physical memory ownership
   - runtime allocator enable gate
   - scheduler/process init
4. Enter scheduler; start init process.

### Memory model

- Before `mem_init_complete`: stack + static globals only.
- After physical memory ownership: page allocator is sole source of dynamic RAM.
- After runtime hook enable: TinyGo heap may allocate through kernel allocator.
- Long-term target: support both physical and virtual memory backends under the
  same allocator contract.

### Execution model

- Kernel runs privileged; user programs run unprivileged.
- Timer interrupt drives preemption.
- Syscalls enter via trap gate, validated, dispatched, and return via trapframe.
- Process state transitions: `UNUSED -> EMBRYO -> RUNNABLE -> RUNNING -> SLEEPING/ZOMBIE`.

### Concurrency model

- Phase 0-7: effectively single-core (second core parked/idle).
- Phase 8+: both cores active with per-core run queues or shared queue + locks.
- All shared mutable kernel structures protected by lock discipline.

### File and device model

- Uniform descriptor interface for devices and files.
- Path resolution + inode layer expose xv6-like semantics.
- Console/UART remain first-class debug and user I/O channels.

## High-level operations documentation

### Process lifecycle (target behavior)

1. `fork` clones parent metadata and address mappings.
2. Child marked `RUNNABLE`; scheduler picks child/parent by tick policy.
3. `exit` closes descriptors, reparents children, marks `ZOMBIE`.
4. `wait` reaps `ZOMBIE`, returns pid/status, releases process resources.

### Trap/syscall flow

1. User executes `syscall` entry sequence.
2. Assembly trap handler saves user registers to trapframe.
3. TinyGo trap dispatcher validates syscall number/args.
4. Handler executes, sets return value, may trigger reschedule.
5. Return path restores registers and resumes user mode.

### Memory allocation flow

1. Early boot uses static storage only.
2. Page allocator initializes over owned physical frames.
3. Runtime allocator requests pages/spans from kernel interface.
4. Future VM layer maps pages into per-process address spaces.

---

This plan intentionally mirrors xv6 concepts (small trusted kernel core, clear
subsystems, incremental complexity) while adapting them for TinyGo, Xtensa, and
ESP32-S3 hardware constraints.
