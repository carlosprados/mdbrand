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

# Uses the unbranded default bundle, so it proves the tool works on a clean
# machine with no brand set up.

# Build examples/demo.md with --brand none
example: build
    ./{{bin}} build examples/demo.md --brand none -o examples/demo.pdf

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
    rm -f {{bin}} examples/demo.pdf
