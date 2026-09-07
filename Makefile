BIN     := mdbrand
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/carlosprados/mdbrand/cmd.Version=$(VERSION)
PREFIX  ?= $(HOME)/.local

.PHONY: build install skill test fmt vet check example clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .

install: build
	install -Dm755 $(BIN) $(PREFIX)/bin/$(BIN)
	@echo "installed $(PREFIX)/bin/$(BIN) ($(VERSION))"

# Symlinks the skill into Claude Code, keeping this repo the single source of
# truth for it.
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

clean:
	rm -f $(BIN) examples/demo.pdf
