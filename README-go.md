# pstree-go

A Go implementation of the classic `pstree` utility that displays running processes as a tree.

## Overview

This is a complete rewrite of the original C `pstree` program in Go, maintaining compatibility with the original command-line interface while adding modern Go features and cross-platform support.

## Features

- **Cross-platform support**: Works on Linux, macOS, FreeBSD, NetBSD, OpenBSD, AIX, and other Unix-like systems
- **Multiple graphics modes**: ASCII, IBM-850, VT100, and UTF-8 tree drawing characters
- **Flexible filtering**: Filter by user, PID, command string, or exclude root processes
- **Direct /proc reading**: On Linux, reads directly from `/proc` filesystem for better performance
- **Terminal width detection**: Automatically adjusts output to terminal width
- **Modern CLI**: Uses Cobra for command-line parsing with help and version support

## Installation

### From Source

```bash
# Clone the repository
git clone <repository-url>
cd pstree

# Build the Go version
make -f Makefile.go build

# Install (optional)
make -f Makefile.go install
```

### Dependencies

- Go 1.21 or later
- `github.com/spf13/cobra` for CLI handling
- `golang.org/x/sys/unix` for system calls

## Usage

```
pstree [flags] [pid ...]

Flags:
  -d, --debug         print debugging info to stderr
  -f, --file string   read input from file (- is stdin)
  -g, --graphics int  graphics chars (0=ASCII, 1=IBM-850, 2=VT100, 3=UTF-8)
  -h, --help          help for pstree
  -l, --level int     print tree to n levels deep (default 100)
  -U, --no-root       don't show branches containing only root processes
  -p, --pid int       show only branches containing process pid (default -1)
  -s, --string string show only branches containing process with string in commandline
  -u, --user string   show only branches containing processes of user
      --version       version for pstree
  -w, --wide          wide output, not truncated to window width
```

## Examples

```bash
# Show all processes
./build/pstree-go

# Show processes for a specific user
./build/pstree-go -u username

# Show processes containing a specific string
./build/pstree-go -s "firefox"

# Show process tree for a specific PID
./build/pstree-go -p 1234

# Use UTF-8 graphics characters
./build/pstree-go -g 3

# Limit tree depth to 3 levels
./build/pstree-go -l 3

# Wide output (no truncation)
./build/pstree-go -w

# Read from file instead of running ps
./build/pstree-go -f process_list.txt
```

## Graphics Modes

- **0 (ASCII)**: Uses basic ASCII characters (`|`, `\`, `-`, `+`)
- **1 (IBM-850)**: Uses IBM-850 box drawing characters
- **2 (VT100)**: Uses VT100 terminal sequences
- **3 (UTF-8)**: Uses Unicode box drawing characters (recommended for modern terminals)

## Process Group Leaders

Process group leaders are marked with `=` in the tree output.

## Differences from Original C Version

### Improvements
- Modern command-line interface with help and version commands
- Better error handling and reporting
- Cross-platform build system with Makefile
- Cleaner, more maintainable code structure
- Built-in dependency management with Go modules

### Compatibility
- All original command-line options are supported
- Output format matches the original as closely as possible
- Same filtering and display logic

## Building

### Standard Build
```bash
make -f Makefile.go build
```

### Cross-Platform Builds
```bash
# Build for all platforms
make -f Makefile.go build-all

# Build for specific platforms
make -f Makefile.go build-linux
make -f Makefile.go build-darwin
make -f Makefile.go build-windows
```

### Development
```bash
# Download dependencies
make -f Makefile.go deps

# Run tests
make -f Makefile.go test

# Clean build artifacts
make -f Makefile.go clean
```

## Platform Support

### Tested Platforms
- Linux (direct /proc reading)
- macOS/Darwin
- FreeBSD, NetBSD, OpenBSD

### Process Information Sources
- **Linux**: Direct `/proc` filesystem reading (preferred) or `ps` command
- **Other Unix**: `ps` command with platform-specific arguments

## Performance

The Go version offers several performance advantages:
- Direct `/proc` reading on Linux eliminates the overhead of spawning `ps`
- Efficient memory management with Go's garbage collector
- Concurrent-safe design for potential future enhancements

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This Go implementation maintains the same GNU General Public License as the original C version.

## Author

Go implementation based on the original C `pstree` by Fred Hucht (c) 1992-2022.
