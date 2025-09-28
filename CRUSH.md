# CRUSH.md - pstree Go Implementation

## Build/Test/Lint Commands
- **Build**: `make build` or `go build -ldflags "-X main.version=3.0.0" -o pstree-go .`
- **Test**: `make test` or `go test -v ./...`
- **Clean**: `make clean`
- **Lint**: `golangci-lint run` or `go vet ./...`
- **Format**: `go fmt ./...`
- **Dependencies**: `make deps` or `go mod tidy`
- **Cross-compile**: `make build-all` (Linux/macOS/Windows)

## Code Style Guidelines
- **Package**: Single main package with supporting files (structs.go, tree.go, terminal.go)
- **Imports**: Standard library first, then third-party (charmbracelet/*, spf13/cobra)
- **Variables**: camelCase for local, PascalCase for exported, ALL_CAPS for constants
- **Types**: PascalCase structs (Process, Config, TreeChars), descriptive field names
- **Functions**: PascalCase for exported, camelCase for internal
- **Error handling**: Return errors, use log.Errorf for user-facing errors
- **Comments**: Minimal, only for exported types/functions or complex logic
- **Constants**: Use iota for enums (GraphicsASCII, GraphicsPC850, etc.)
- **CLI**: Use cobra.Command with flags, maintain compatibility with original C version
- **Logging**: Use charmbracelet/log, debug level controlled by -d flag
- **Cross-platform**: Use runtime.GOOS checks, separate Linux /proc implementation

## Architecture
- main.go: CLI setup and entry point
- structs.go: Core types (Process, Config, TreeChars)  
- tree.go: Tree building and rendering logic
- terminal.go: Terminal width detection and display