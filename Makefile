TINYGO ?= tinygo
TARGET ?= ./targets/esp32s3-windmills.json
OUT ?= build/windmills.elf
# Fixed at 2024-01-01T00:00:00Z for reproducible image metadata.
SOURCE_DATE_EPOCH ?= 1704067200

.PHONY: firmware clean test

firmware:
	mkdir -p build
	TZ=UTC LC_ALL=C SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) \
		$(TINYGO) build -target $(TARGET) -scheduler=none -gc=none -panic=trap -opt=2 -o $(OUT) ./cmd/kernel

clean:
	rm -rf build

test:
	go test ./...
