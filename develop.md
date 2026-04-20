# Development board run guide (USB)

This project targets an ESP32-S3 board connected over USB.

## Prerequisites

- TinyGo installed and on `PATH`
- Python 3 installed and on `PATH`
- ESP flashing tooling (`esptool.py`)
- Python serial monitor module (`pyserial`, for `python3 -m serial.tools.miniterm`)
- A USB data cable and an ESP32-S3 development board

Install Python tools if needed:

```bash
python3 -m pip install --user esptool pyserial
```

## Build firmware

```bash
cd <path-to-windmills>
make firmware
```

This produces `build/windmills.bin`.

## Flash to board

The `PORT` variable defaults to `/dev/ttyACM0` (standard Linux USB-serial path for ESP32-S3).
Override it if your board appears on a different node:

```bash
make flash
# or, for a different port:
make flash PORT=/dev/ttyUSB0
```

`make flash` uses `tinygo flash` (not a raw `esptool write-flash` at `0x0`), so TinyGo handles the proper ESP32-S3 image layout and offsets.

## Observe serial output

Open a serial monitor at `115200` baud and reset the board.

```bash
make monitor
# or, for a different port:
make monitor PORT=/dev/ttyUSB0
```

The monitor command prefers `./venv/bin/python` when present, so it will use your venv-installed `pyserial` automatically.

Expected Phase 1 boot messages:

- `windmills: phase0 boot`
- `windmills: phase1 init`
- `windmills: phase2 init`
- `windmills: kthread0 ran`
- `windmills: entering halt loop`

## Phase 1 hardware validation checklist

- Board boots and reaches `kmain` output reliably.
- Trap/vector symbols are linked (`windmills_vector_table`, `windmills_trap_entry`, `windmills_trap_exit`).
- Timer interrupt path increments the monotonic tick counter.
- Cooperative scheduler executes `kthread0` exactly once.

## Current Phase 1 expected board behavior

- Yes: this should boot on ESP32-S3 and run the single-thread Phase 1 path.
- Expected serial output after reset:
  - `windmills: phase0 boot`
  - `windmills: phase1 init`
  - `windmills: kthread0 ran`
  - `windmills: entering halt loop`
- After printing the above, the kernel intentionally stays in the halt loop.
- Current limitation: timer/trap plumbing is still skeleton-level; this phase does
  not yet provide full preemptive ISR-driven scheduling behavior.

## Code review summary

### What looks solid

- The repository has focused host tests (`go test ./...`) for scheduler/interrupt
  skeleton behavior and Phase 2 memory ownership invariants.
- `Makefile` firmware builds are deterministic (`SOURCE_DATE_EPOCH`, `TZ`, `LC_ALL`).
- Memory allocation paths guard against reserved-region allocation by design and test.

### Gaps and risks to track

- Hardware-in-the-loop tests are still manual; no automated board test harness exists yet.
- Trap/vector and timer code is explicitly skeleton-level, so ISR-driven preemption is
  not implemented yet.
- On this environment, host tests pass but firmware build requires installing TinyGo
  first (`tinygo: not found` if missing).

### Can this be tested on an ESP32-S3 dev board now?

Yes. The current codebase is testable on an ESP32-S3 dev board for bring-up validation:
build firmware, flash it, and confirm the expected UART boot messages shown above.
