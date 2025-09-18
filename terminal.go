package pstree

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

func calculateTerminalWidth() {
	// Get terminal width
	config.Columns = getTerminalWidth()
	if config.Columns == 0 {
		config.Columns = maxLine - 1
	}

	// Add space for graphics characters
	config.Columns += len(config.TreeChar.SG) + len(config.TreeChar.EG)
	if config.Columns >= maxLine {
		config.Columns = maxLine - 1
	}

	log.Infof("columns: %d", config.Columns)
}

func getTerminalWidthSTTY() (int, error) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	parts := strings.Fields(string(out))
	if len(parts) == 2 {
		if c, err := strconv.Atoi(parts[1]); err == nil {
			return c, nil
		}
	}
	return 0, errors.New("terminal width using stty cannot be determined")
}

// getTerminalWidth gets the terminal width
func getTerminalWidth() int {

	if config.WOption {
		return maxLine - 1
	}

	// Try to get terminal size

	// method 1 : term pkg
	if width, _, err := term.GetSize(os.Stdout.Fd()); err == nil {
		return width
	}
	// method 2 : unix pkg
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil {
		return int(ws.Col)
	}
	// method 3: call stty
	if width, err := getTerminalWidthSTTY(); err == nil {
		return width
	}
	// method 4: env variable
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if c, err := strconv.Atoi(cols); err == nil {
			return c
		}
	}

	return 80 // default
}
