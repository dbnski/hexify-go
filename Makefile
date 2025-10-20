GO      := go
GOFLAGS :=
TARGET  := hexify
SRC     := hexify.go
TESTS   := $(wildcard test/*.in)

.PHONY: all test clean

all: $(TARGET)

$(TARGET): $(SRC)
	$(GO) build $(GOLAGS) -o $@ $^

test: all
	@for test in $(TESTS); do \
		echo "Running $$test..."; \
		bash test/test.sh $$test || { echo "Test $$test failed."; exit 1; }; \
	done
	@echo "All tests passed."

clean:
	rm -f $(TARGET)
