BIN     := mdbrand
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/carlosprados/mdbrand/cmd.Version=$(VERSION)
PREFIX  ?= $(HOME)/.local

.PHONY: build install skill test fmt vet check example torture clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

install: build
	install -Dm755 $(BIN) $(PREFIX)/bin/$(BIN)
	@echo "installed $(PREFIX)/bin/$(BIN) ($(VERSION))"

# Symlinks the skill into Claude Code, keeping this checkout the single source
# of truth while developing it. Everyone else installs the copy embedded in the
# binary, with `mdbrand skill install`, which needs no checkout at all.
SKILL_DIR ?= $(HOME)/.claude/skills/mdbrand
skill:
	@mkdir -p $(SKILL_DIR)
	@ln -sfn $(CURDIR)/SKILL.md $(SKILL_DIR)/SKILL.md
	@echo "linked $(SKILL_DIR)/SKILL.md -> $(CURDIR)/SKILL.md"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: fmt vet test

# Builds the bundled example with the unbranded default bundle, so it works on
# a clean machine with no brand set up.
example: build
	./$(BIN) build examples/demo.md --brand none -o examples/demo.pdf

# Builds the fixture documents and reads the PDFs and logs they produce. The
# unit tests cover the arithmetic; this covers the artifact, which is where
# every defect this tool has shipped actually lived. Needs the full toolchain.
torture: build
	@scripts/torture.sh

clean:
	rm -f $(BIN) examples/demo.pdf
