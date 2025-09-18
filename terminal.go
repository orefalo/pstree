package main

import (
	"os"
	"strconv"

	"github.com/charmbracelet/log"
	"github.com/charmbracelet/x/term"
)

const (
	maxLine = 8192
)

func CalculateTerminalWidth() {
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

	// method 2: env variable
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if c, err := strconv.Atoi(cols); err == nil {
			return c
		}
	}

	return 80 // default
}
