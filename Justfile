bin     := "mdbrand"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X github.com/carlosprados/mdbrand/cmd.Version=" + version
prefix  := env_var_or_default("PREFIX", env_var("HOME") + "/.local")

# Symlinks the skill into Claude Code, keeping this checkout the single source
# of truth while developing it. Everyone else installs the copy embedded in the
# binary, with `mdbrand skill install`, which needs no checkout at all.
skill_dir := env_var_or_default("SKILL_DIR", env_var("HOME") + "/.claude/skills/mdbrand")

# List the recipes
default:
    @just --list

# Build the binary, version from git describe
build:
    go build -ldflags "{{ldflags}}" -o {{bin}} .

# Build and install into $PREFIX/bin (default ~/.local/bin)
install: build
    install -Dm755 {{bin}} {{prefix}}/bin/{{bin}}
    @echo "installed {{prefix}}/bin/{{bin}} ({{version}})"

# Dev symlink of SKILL.md into ~/.claude/skills/mdbrand
skill:
    @mkdir -p {{skill_dir}}
    @ln -sfn {{justfile_directory()}}/SKILL.md {{skill_dir}}/SKILL.md
    @echo "linked {{skill_dir}}/SKILL.md -> {{justfile_directory()}}/SKILL.md"

# Unit tests
test:
    go test ./...

# Rewrite testdata/contract after a deliberate change to a key, flag or status
contract:
    MDBRAND_UPDATE_CONTRACT=1 go test ./... -run Contract
    git diff --stat testdata/contract

# gofmt -w .
fmt:
    gofmt -w .

# go vet
vet:
    go vet ./...

# golangci-lint: import direction, dropped errors, complexity (.golangci.yml)
lint:
    golangci-lint run ./...

# gofmt, vet, lint and the unit tests
check: fmt vet lint test

# Writes every example out of the binary, as an agent would get it, and builds
# it there. They all say brand: none, so this proves the tool works on a clean
# machine with no bundle set up, and an example that stops building fails CI.
example: build
    #!/usr/bin/env bash
    set -euo pipefail
    out="$(mktemp -d)"; trap 'rm -rf "$out"' EXIT
    for name in $(./{{bin}} example | awk '/^[a-z]+  /{print $1}'); do
        ./{{bin}} example "$name" "$out/$name" >/dev/null
        ./{{bin}} build "$out/$name/$name.md" -q
    done

# The unit tests cover the arithmetic; this covers the artifact, which is where
# every defect this tool has shipped actually lived. Needs the full toolchain.

# Build testdata/ and read the PDFs and logs that come out
torture: build
    @scripts/torture.sh

# Pictures are regenerated from the examples so they show what the tool prints
# today. Needs freeze, vhs, ffmpeg and ImageMagick besides the toolchain.

# Regenerate the README pictures in assets/readme
shots: build
    @scripts/shots.sh

# Remove the binary and the built example
clean:
    rm -f {{bin}} examples/*/*.pdf examples/*/*.docx
