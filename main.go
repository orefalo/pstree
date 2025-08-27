package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

const (
	maxLine = 8192
	version = "3.0.0"
)

// TreeChars defines the characters used for drawing the tree
type TreeChars struct {
	S2   string // String between header and pid
	P    string // dito, when parent of printed children
	PGL  string // Process group leader
	NPGL string // No process group leader
	BarC string // bar for line with child
	Bar  string // bar for line without child
	BarL string // bar for last child
	SG   string // Start graphics (alt char set)
	EG   string // End graphics (alt char set)
	Init string // Init string sent at the beginning
}

// Graphics modes
const (
	GraphicsASCII = iota
	GraphicsPC850
	GraphicsVT100
	GraphicsUTF8
)

var treeChars = []TreeChars{
	// ASCII
	{"--", "-+", "=", "-", "|", "|", "\\", "", "", ""},
	// PC850
	{"\304\304", "\304\302", "\372", "\304", "\303", "\263", "\300", "", "", ""},
	// VT100
	{"qq", "qw", "`", "q", "t", "x", "m", "\016", "\017", "\033(B\033)0"},
	// UTF8
	{"\342\224\200\342\224\200", "\342\224\200\342\224\254", "=", "\342\224\200", "\342\224\234", "\342\224\202", "\342\224\224", "", "", ""},
}

// Process represents a single process
type Process struct {
	UID     int64
	PID     int64
	PPID    int64
	PGID    int64
	Name    string
	Cmd     string
	Print   bool
	Parent  int
	Child   int
	Sister  int
	ThCount int64
}

// Config holds the application configuration
type Config struct {
	ShowAll   bool
	SOption   bool
	UOption   bool
	Name      string
	Str       string
	IPid      int64
	Input     string
	AtLDepth  int
	MaxLDepth int
	Compress  bool
	Debug     bool
	Graphics  int
	Wide      bool
	Columns   int
	TreeChar  *TreeChars
}

var (
	config  Config
	procs   []Process
	myPID   int
	rootPID int64
	nProc   int
)

func init() {
	config = Config{
		ShowAll:   true,
		MaxLDepth: 100,
		Graphics:  GraphicsASCII,
		TreeChar:  &treeChars[GraphicsASCII],
	}
	myPID = os.Getpid()
}

// getProcessesDirect reads processes directly from /proc filesystem (Linux)
func getProcessesDirect() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("direct process reading only supported on Linux")
	}

	procDirs, err := filepath.Glob("/proc/[0-9]*")
	if err != nil {
		return err
	}

	procs = make([]Process, 0, len(procDirs))

	for _, procDir := range procDirs {
		var proc Process

		// Get UID from directory stat
		if stat, err := os.Stat(procDir); err == nil {
			if sysStat, ok := stat.Sys().(*syscall.Stat_t); ok {
				proc.UID = int64(sysStat.Uid)
				if u, err := user.LookupId(strconv.Itoa(int(proc.UID))); err == nil {
					proc.Name = u.Username
				} else {
					proc.Name = fmt.Sprintf("#%d", proc.UID)
				}
			}
		} else {
			continue // process vanished
		}

		// Read /proc/PID/stat
		statPath := filepath.Join(procDir, "stat")
		statData, err := os.ReadFile(statPath)
		if err != nil {
			continue // process vanished
		}

		statFields := strings.Fields(string(statData))
		if len(statFields) < 5 {
			continue
		}

		if pid, err := strconv.ParseInt(statFields[0], 10, 64); err == nil {
			proc.PID = pid
		} else {
			continue
		}

		proc.Cmd = strings.Trim(statFields[1], "()")

		if ppid, err := strconv.ParseInt(statFields[3], 10, 64); err == nil {
			proc.PPID = ppid
		}

		if pgid, err := strconv.ParseInt(statFields[4], 10, 64); err == nil {
			proc.PGID = pgid
		}

		proc.ThCount = 1

		// Read /proc/PID/cmdline for full command
		cmdlinePath := filepath.Join(procDir, "cmdline")
		if cmdlineData, err := os.ReadFile(cmdlinePath); err == nil && len(cmdlineData) > 0 {
			// Replace null bytes with spaces
			cmdline := strings.ReplaceAll(string(cmdlineData), "\x00", " ")
			cmdline = strings.TrimSpace(cmdline)
			if cmdline != "" {
				proc.Cmd = cmdline
			}
		}

		proc.Parent = -1
		proc.Child = -1
		proc.Sister = -1
		proc.Print = false

		procs = append(procs, proc)
	}

	nProc = len(procs)
	return nil
}

// getProcesses reads processes using ps command
func getProcesses() error {
	var cmd *exec.Cmd
	var scanner *bufio.Scanner

	if config.Input != "" {
		if config.Input == "-" {
			scanner = bufio.NewScanner(os.Stdin)
		} else {
			file, err := os.Open(config.Input)
			if err != nil {
				return err
			}
			defer file.Close()
			scanner = bufio.NewScanner(file)
		}
	} else {
		// Use ps command based on OS
		var psCmd []string
		switch runtime.GOOS {
		case "linux":
			psCmd = []string{"ps", "-eo", "uid,pid,ppid,pgid,args"}
		case "darwin", "freebsd", "netbsd", "openbsd":
			psCmd = []string{"ps", "-axwwo", "user,pid,ppid,pgid,command"}
		case "aix":
			psCmd = []string{"ps", "-eko", "uid,pid,ppid,pgid,thcount,args"}
		default:
			psCmd = []string{"ps", "-ef"}
		}

		cmd = exec.Command(psCmd[0], psCmd[1:]...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}

		if err := cmd.Start(); err != nil {
			return err
		}
		defer cmd.Wait()

		scanner = bufio.NewScanner(stdout)
	}

	procs = make([]Process, 0)

	// Skip header line
	if !scanner.Scan() {
		return fmt.Errorf("no input")
	}

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		var proc Process
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Parse based on OS and ps format
		switch runtime.GOOS {
		case "linux", "aix":
			if uid, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
				proc.UID = uid
				if u, err := user.LookupId(fields[0]); err == nil {
					proc.Name = u.Username
				} else {
					proc.Name = fmt.Sprintf("#%s", fields[0])
				}
			}
			if pid, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				proc.PPID = ppid
			}
			if pgid, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
				proc.PGID = pgid
			}
			if len(fields) > 4 {
				if runtime.GOOS == "aix" && len(fields) > 5 {
					if thcount, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
						proc.ThCount = thcount
					}
					proc.Cmd = strings.Join(fields[5:], " ")
				} else {
					proc.ThCount = 1
					proc.Cmd = strings.Join(fields[4:], " ")
				}
			}
		case "darwin", "freebsd", "netbsd", "openbsd":
			proc.Name = fields[0]
			if pid, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				proc.PPID = ppid
			}
			if pgid, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
				proc.PGID = pgid
			}
			if len(fields) > 4 {
				proc.Cmd = strings.Join(fields[4:], " ")
			}
			proc.ThCount = 1
		default:
			// Default ps -ef format
			proc.Name = fields[0]
			if pid, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				proc.PPID = ppid
			}
			if len(fields) > 7 {
				proc.Cmd = strings.Join(fields[7:], " ")
			}
			proc.ThCount = 1
		}

		proc.Parent = -1
		proc.Child = -1
		proc.Sister = -1
		proc.Print = false

		procs = append(procs, proc)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	nProc = len(procs)
	return nil
}

// getRootPID finds the root process PID
func getRootPID() int64 {
	// Look for PID 1
	for _, proc := range procs {
		if proc.PID == 1 {
			return proc.PID
		}
	}

	// Look for PPID 0
	for _, proc := range procs {
		if proc.PPID == 0 {
			return proc.PID
		}
	}

	// Look for PPID 1
	for _, proc := range procs {
		if proc.PPID == 1 {
			return proc.PID
		}
	}

	// Look for PID == PPID
	for _, proc := range procs {
		if proc.PID == proc.PPID {
			return proc.PID
		}
	}

	fmt.Fprintf(os.Stderr, "pstree: No process found with PID == 1 || PPID == 0 || PPID == 1 || PID == PPID\n")
	os.Exit(1)
	return 0
}

// getPidIndex finds the index of a process by PID
func getPidIndex(pid int64) int {
	for i := len(procs) - 1; i >= 0; i-- {
		if procs[i].PID == pid {
			return i
		}
	}
	return -1
}

// makeTree builds the process hierarchy
func makeTree() {
	for i := range procs {
		parentIdx := getPidIndex(procs[i].PPID)
		if parentIdx != i && parentIdx != -1 {
			procs[i].Parent = parentIdx
			if procs[parentIdx].Child == -1 {
				procs[parentIdx].Child = i
			} else {
				sister := procs[parentIdx].Child
				for procs[sister].Sister != -1 {
					sister = procs[sister].Sister
				}
				procs[sister].Sister = i
			}
		}
	}
}

// markChildren recursively marks children for printing
func markChildren(idx int) {
	procs[idx].Print = true
	child := procs[idx].Child
	for child != -1 {
		markChildren(child)
		child = procs[child].Sister
	}
}

// markProcs marks processes for printing based on criteria
func markProcs() {
	for i := range procs {
		if config.ShowAll {
			procs[i].Print = true
		} else {
			shouldMark := false

			// Check various criteria
			if config.Name != "" && procs[i].Name == config.Name {
				shouldMark = true
			}
			if config.UOption && procs[i].Name != "root" {
				shouldMark = true
			}
			if config.IPid != -1 && procs[i].PID == config.IPid {
				shouldMark = true
			}
			if config.SOption && strings.Contains(procs[i].Cmd, config.Str) && procs[i].PID != int64(myPID) {
				shouldMark = true
			}

			if shouldMark {
				// Mark parents
				parent := procs[i].Parent
				for parent != -1 {
					procs[parent].Print = true
					parent = procs[parent].Parent
				}
				// Mark children
				markChildren(i)
			}
		}
	}
}

// dropProcs removes processes that won't be printed from the tree structure
func dropProcs() {
	for i := range procs {
		if procs[i].Print {
			// Drop children that won't print
			child := procs[i].Child
			for child != -1 && !procs[child].Print {
				child = procs[child].Sister
			}
			procs[i].Child = child

			// Drop sisters that won't print
			sister := procs[i].Sister
			for sister != -1 && !procs[sister].Print {
				sister = procs[sister].Sister
			}
			procs[i].Sister = sister
		}
	}
}

// printTree recursively prints the process tree
func printTree(idx int, head string) {
	if head == "" && !procs[idx].Print {
		return
	}

	if config.AtLDepth == config.MaxLDepth {
		return
	}
	config.AtLDepth++

	var thread string
	if procs[idx].ThCount > 1 {
		thread = fmt.Sprintf("[%d]", procs[idx].ThCount)
	}

	var pgl string
	if procs[idx].PID == procs[idx].PGID {
		pgl = config.TreeChar.PGL
	} else {
		pgl = config.TreeChar.NPGL
	}

	var barChar string
	if head == "" {
		barChar = ""
	} else if procs[idx].Sister != -1 {
		barChar = config.TreeChar.BarC
	} else {
		barChar = config.TreeChar.BarL
	}

	var pChar string
	if procs[idx].Child != -1 {
		pChar = config.TreeChar.P
	} else {
		pChar = config.TreeChar.S2
	}

	out := fmt.Sprintf("%s%s%s%s%s%s %05d %s %s%s",
		config.TreeChar.SG,
		head,
		barChar,
		pChar,
		pgl,
		config.TreeChar.EG,
		procs[idx].PID,
		procs[idx].Name,
		thread,
		procs[idx].Cmd)

	if len(out) > config.Columns-1 {
		out = out[:config.Columns-1]
	}
	fmt.Println(out)

	// Process children
	var nhead string
	if head == "" {
		nhead = ""
	} else if procs[idx].Sister != -1 {
		nhead = head + config.TreeChar.Bar + " "
	} else {
		nhead = head + "  "
	}

	child := procs[idx].Child
	for child != -1 {
		printTree(child, nhead)
		child = procs[child].Sister
	}

	config.AtLDepth--
}

// getTerminalWidth gets the terminal width
func getTerminalWidth() int {
	if config.Wide {
		return maxLine - 1
	}

	// Try to get terminal size
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil {
		return int(ws.Col)
	}

	// Fallback to environment variable
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if c, err := strconv.Atoi(cols); err == nil {
			return c
		}
	}

	return 80 // default
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "pstree [flags] [pid ...]",
		Short: "Display running processes as a tree",
		Long: `pstree shows running processes as a tree. The tree is rooted at either pid or init if pid is omitted.
If a user name is specified, all process trees rooted at processes owned by that user are shown.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize graphics
			if config.Graphics < 0 || config.Graphics >= len(treeChars) {
				return fmt.Errorf("invalid graphics parameter")
			}
			config.TreeChar = &treeChars[config.Graphics]

			// Validate user if specified
			if config.Name != "" {
				if _, err := user.Lookup(config.Name); err != nil {
					return fmt.Errorf("user '%s' does not exist", config.Name)
				}
				config.ShowAll = false
			}

			// Set other options
			if config.UOption || config.SOption || config.IPid != -1 {
				config.ShowAll = false
			}
			if config.Str != "" {
				config.SOption = true
				config.ShowAll = false
			}

			// Get processes
			var err error
			if runtime.GOOS == "linux" && config.Input == "" {
				err = getProcessesDirect()
			} else {
				err = getProcesses()
			}
			if err != nil {
				return err
			}

			if nProc == 0 {
				return fmt.Errorf("no processes read")
			}

			// Find root PID
			rootPID = getRootPID()

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

			// Print initialization string
			fmt.Print(config.TreeChar.Init)

			// Build and print tree
			makeTree()
			markProcs()
			dropProcs()

			if len(args) == 0 {
				// No specific PIDs, start from root
				rootIdx := getPidIndex(rootPID)
				if rootIdx != -1 {
					printTree(rootIdx, "")
				}
			} else {
				// Print trees for specified PIDs
				for _, arg := range args {
					if pid, err := strconv.ParseInt(arg, 10, 64); err == nil {
						if idx := getPidIndex(pid); idx != -1 {
							printTree(idx, "")
						}
					}
				}
			}

			return nil
		},
	}

	// Add flags
	rootCmd.Flags().StringVarP(&config.Input, "file", "f", "", "read input from file (- is stdin)")
	rootCmd.Flags().IntVarP(&config.Graphics, "graphics", "g", GraphicsASCII, "graphics chars (0=ASCII, 1=IBM-850, 2=VT100, 3=UTF-8)")
	rootCmd.Flags().IntVarP(&config.MaxLDepth, "level", "l", 100, "print tree to n levels deep")
	rootCmd.Flags().StringVarP(&config.Name, "user", "u", "", "show only branches containing processes of user")
	rootCmd.Flags().BoolVarP(&config.UOption, "no-root", "U", false, "don't show branches containing only root processes")
	rootCmd.Flags().StringVarP(&config.Str, "string", "s", "", "show only branches containing process with string in commandline")
	rootCmd.Flags().Int64VarP(&config.IPid, "pid", "p", -1, "show only branches containing process pid")
	rootCmd.Flags().BoolVarP(&config.Wide, "wide", "w", false, "wide output, not truncated to window width")
	rootCmd.Flags().BoolVarP(&config.Debug, "debug", "d", false, "print debugging info to stderr")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
