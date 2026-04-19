TINYGO ?= tinygo
TARGET ?= ./targets/esp32s3-windmills.json
OUT ?= build/windmills.bin
ESPTOOL ?= esptool
PYTHON ?= python3
PORT ?= /dev/cu.usbmodem1101
FLASH_BAUD ?= 921600
MONITOR_BAUD ?= 115200
# Fixed at 2024-01-01T00:00:00Z for reproducible image metadata.
SOURCE_DATE_EPOCH ?= 1704067200

.PHONY: firmware flash monitor clean test

firmware:
	mkdir -p build
	TZ=UTC LC_ALL=C SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) \
		$(TINYGO) build -target $(TARGET) -scheduler=none -gc=none -panic=trap -opt=2 -o $(OUT) ./cmd/kernel

flash: firmware
	$(ESPTOOL) --chip esp32s3 --port $(PORT) --baud $(FLASH_BAUD) --before default-reset --after hard-reset write-flash 0x0 $(OUT)

monitor:
	$(PYTHON) -m serial.tools.miniterm $(PORT) $(MONITOR_BAUD)

clean:
	rm -rf build

test:
	go test ./...
