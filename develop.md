# Development board run guide (USB)

This project targets an ESP32-S3 board connected over USB.

## Prerequisites

- TinyGo installed and on `PATH`
- ESP flashing tooling (`esptool.py`)
- A USB data cable and an ESP32-S3 development board

## Build firmware

```bash
cd /home/runner/work/windmills/windmills
make firmware
```

This produces `/home/runner/work/windmills/windmills/build/windmills.elf`.

## Flash to board

Use your board's serial device (example: `/dev/ttyACM0`):

```bash
esptool.py --chip esp32s3 --port /dev/ttyACM0 --baud 921600 --before default_reset --after hard_reset write_flash 0x0 /home/runner/work/windmills/windmills/build/windmills.elf
```

## Observe serial output

Open a serial monitor at `115200` baud and reset the board.

Expected Phase 1 boot messages:

- `windmills: phase0 boot`
- `windmills: phase1 init`
- `windmills: kthread0 ran`
- `windmills: entering halt loop`

## Phase 1 hardware validation checklist

- Board boots and reaches `kmain` output reliably.
- Trap/vector symbols are linked (`windmills_vector_table`, `windmills_trap_entry`, `windmills_trap_exit`).
- Timer interrupt path increments the monotonic tick counter.
- Cooperative scheduler executes `kthread0` exactly once.
