package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	maxLine = 8192
	version = "1.0.0"
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
	UID         int
	PID         int
	PPID        int
	PGID        int
	Owner       string
	Cmd         string
	ThreadCount int

	// line prints when true
	Print bool
	// meta data to create and filter the tree structure
	ParentIdx int
	ChildIdx  int
	SisterIdx int
}

// Config holds the application configuration
type Config struct {
	// show all processes
	AOption bool
	// filter on a given user
	UOption bool
	// show pids in the rendering
	POption bool
	// debug option
	DOption bool
	// filter processes on this owner
	SearchOwner string
	// optional string to filter start processes
	SearchStr string
	// optional pid to start from, default parent pid
	SearchPid int

	//Input string

	// maximum tree depth
	MaxLDepth int

	// TODO: Compress output
	Compress bool
	//Debug    bool
	// character set selector in treeChars
	Graphics int
	// For long output (no width truncation)
	Long bool
	// terminal width in columns
	Columns int
	// character set used to render the tree
	TreeChar *TreeChars
}

var (
	// This holds the command line options
	config Config

	// This holds the output of 'ps'
	procs []Process
	// number of discovered processes
	// TODO: why is this not procs.length
	nProc int

	// that's mypid
	myPID int

	// and my parent pid
	myPPID int

	// what pid is the rendering starting from
	rootPID int

	// current rendering depth
	AtLDepth int = 0
)

// getTopPID finds the root process PID
func getTopPID() int {

	if config.SearchPid != -1 {
		return config.SearchPid
	}

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
func getPidIndex(pid int) int {
	for i := len(procs) - 1; i >= 0; i-- {
		if procs[i].PID == pid {
			if pid != 1 {
				log.Debugf("getPidIndex(%d)=%d\n", pid, i)
			}
			return i
		}
	}
	log.Debugf("getPidIndex(%d)=-1\n", pid)
	return -1
}

// makeTreeHierarchy builds the process hierarchy
func makeTreeHierarchy() {
	for i := range procs {
		parentIdx := getPidIndex(procs[i].PPID)
		if parentIdx != i && parentIdx != -1 {
			procs[i].ParentIdx = parentIdx
			if procs[parentIdx].ChildIdx == -1 {
				procs[parentIdx].ChildIdx = i
			} else {
				sister := procs[parentIdx].ChildIdx
				for procs[sister].SisterIdx != -1 {
					sister = procs[sister].SisterIdx
				}
				procs[sister].SisterIdx = i
			}
		}
	}
}

func debugPrintProcs(enforcePrintFlag bool) {

	var (
		purple    = lipgloss.Color("99")
		gray      = lipgloss.Color("245")
		lightGray = lipgloss.Color("241")

		headerStyle  = lipgloss.NewStyle().Foreground(purple).Bold(true).Align(lipgloss.Center)
		cellStyle    = lipgloss.NewStyle().Padding(0, 1).Width(30)
		oddRowStyle  = cellStyle.Foreground(gray)
		evenRowStyle = cellStyle.Foreground(lightGray)
	)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(purple)).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle
			case row%2 == 0:
				return evenRowStyle
			default:
				return oddRowStyle
			}
		}).
		Headers("idx", "parentIdx", "childIdx", "PID", "PPID", "PROCESS")

	for i := range procs {
		p := procs[i]
		if enforcePrintFlag && p.Print {
			t.Row(strconv.Itoa(i), strconv.Itoa(p.ParentIdx), strconv.Itoa(p.ChildIdx), strconv.Itoa(p.PID), strconv.Itoa(p.PPID), p.Cmd)
		}
	}
	log.Debug(t)
}

// markChildren recursively marks children for printing
func markChildren(idx int) {
	procs[idx].Print = true
	child := procs[idx].ChildIdx
	for child != -1 {
		markChildren(child)
		child = procs[child].SisterIdx
	}
}

// markProcs marks processes for printing based on criteria
func markProcs() {
	for i := range procs {
		process := procs[i]
		if config.AOption {
			process.Print = true
		} else {
			shouldPrintBranch := false

			// Check various criteria
			if config.SearchOwner != "" && process.Owner == config.SearchOwner {
				shouldPrintBranch = true
			}
			if config.UOption && process.Owner != "root" {
				shouldPrintBranch = true
			}
			if config.SearchPid != -1 && process.PID == config.SearchPid {
				shouldPrintBranch = true
			}
			if config.SearchStr != "" && strings.Contains(process.Cmd, config.SearchStr) && process.PID != myPID {
				shouldPrintBranch = true
			}

			if shouldPrintBranch {
				// Mark the branch for printing
				parent := process.ParentIdx
				for parent != -1 {
					procs[parent].Print = true
					parent = procs[parent].ParentIdx
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
		process := procs[i]
		if process.Print {
			// Drop children that won't print
			child := process.ChildIdx
			for child != -1 && !procs[child].Print {
				child = procs[child].SisterIdx
			}
			process.ChildIdx = child

			// Drop sisters that won't print
			tmp := process.SisterIdx
			for tmp != -1 && !procs[tmp].Print {
				tmp = procs[tmp].SisterIdx
			}
			process.SisterIdx = tmp
		}
	}
}

// printTree recursively prints the process tree
func printTree(idx int, head string) {
	if head == "" && !procs[idx].Print {
		return
	}

	if AtLDepth == config.MaxLDepth {
		return
	}

	AtLDepth++

	var thread string
	if procs[idx].ThreadCount > 1 {
		thread = fmt.Sprintf("[%d]", procs[idx].ThreadCount)
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
	} else if procs[idx].SisterIdx != -1 {
		barChar = config.TreeChar.BarC
	} else {
		barChar = config.TreeChar.BarL
	}

	var pChar string
	if procs[idx].ChildIdx != -1 {
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
		procs[idx].Owner,
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
	} else if procs[idx].SisterIdx != -1 {
		nhead = head + config.TreeChar.Bar + " "
	} else {
		nhead = head + "  "
	}

	// recursively process children
	child := procs[idx].ChildIdx
	for child != -1 {
		printTree(child, nhead)
		child = procs[child].SisterIdx
	}

	AtLDepth--
}

func main() {

	log.Info("main()")

	var rootCmd = &cobra.Command{
		Use:   "pstree [flags] [pid ...]",
		Short: "Display running processes as a tree",
		Long: `pstree shows running processes as a tree. The tree is rooted at either pid or init if pid is omitted.
If a user name is specified, all process trees rooted at processes owned by that user are shown.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {

			log.Infof("DOption %v", config.DOption)
			if config.DOption {
				log.SetLevel(log.DebugLevel)
				log.Debugf("H1")
			}

			if len(args) == 1 {
				if c, err := strconv.Atoi(args[0]); err == nil {
					config.SearchStr = ""
					config.SearchPid = c
				} else {
					log.Infof("args[0] = %s", args[0])
					config.SearchStr = args[0]
					config.SearchPid = -1
				}
			}

			if config.SearchPid == -1 {
				// default top pid to the parent pid
				config.SearchPid = myPPID
			}
			log.Infof("config.SearchPid = %d", config.SearchPid)

			// Initialize graphics
			if config.Graphics < 0 || config.Graphics >= len(treeChars) {
				log.Errorf("invalid graphics parameter")
				return nil
			}
			config.TreeChar = &treeChars[config.Graphics]

			if config.AOption {
				config.SearchOwner = ""
				config.SearchPid = -1
			}

			// Validate user if specified
			if config.SearchOwner != "" {
				if _, err := user.Lookup(config.SearchOwner); err != nil {
					log.Errorf("user '%s' does not exist", config.SearchOwner)
					return err
				}
				config.AOption = false
			}

			// Get processes
			var err error
			if runtime.GOOS == "linux" {
				err = getProcessesDirect()
			} else {
				err = getProcesses()
			}
			if err != nil {
				return err
			}

			log.Debugf("nProcs = %d", nProc)

			if nProc == 0 {
				log.Errorf("no processes read")
				return nil
			}

			// if we are filtering of a pid, ensure th epid exist.
			// otherwise, if not found, it's a string
			if config.SearchPid != -1 {
				if getPidIndex(config.SearchPid) == -1 {
					// pid not found, it's a string search
					config.SearchStr = args[0]
					config.SearchPid = -1
				}
			}

			calculateTerminalWidth()

			// Print initialization string
			fmt.Print(config.TreeChar.Init)

			// Build and print tree
			makeTreeHierarchy()
			debugPrintProcs(false)
			markProcs()
			debugPrintProcs(true)
			dropProcs()
			debugPrintProcs(true)

			// Find top PID
			rootPID = getTopPID()
			rootIdx := getPidIndex(rootPID)
			if rootIdx != -1 {
				printTree(rootIdx, "")
			}

			//
			//if len(args) == 0 {
			//	// No specific PIDs, start from root
			//	rootIdx := getPidIndex(rootPID)
			//	if rootIdx != -1 {
			//		printTree(rootIdx, "")
			//	}
			//} else {
			//	// Print trees for specified PIDs
			//	for _, arg := range args {
			//		if pid, err := strconv.Atoi(arg); err == nil {
			//			if idx := getPidIndex(pid); idx != -1 {
			//				printTree(idx, "")
			//			}
			//		}
			//
			//	}
			//}

			return nil
		},
	}

	// Add flags
	rootCmd.Flags().StringVarP(&config.SearchOwner, "user", "u", getCurrentUsername(), "show only branches containing processes of user")
	rootCmd.Flags().BoolVarP(&config.UOption, "no-root", "U", false, "don't show branches containing only root processes")
	rootCmd.Flags().BoolVarP(&config.POption, "show-pids", "p", false, "show process pids")
	rootCmd.Flags().IntVarP(&config.MaxLDepth, "level", "l", 100, "print tree to n levels deep")
	rootCmd.Flags().BoolVarP(&config.AOption, "all", "a", false, "show all processes")
	rootCmd.Flags().BoolVarP(&config.Long, "wide", "w", false, "wide output, not truncated to window width")
	rootCmd.Flags().BoolVarP(&config.DOption, "debug", "d", false, "print debugging info to stderr")
	rootCmd.Flags().IntVarP(&config.Graphics, "graphics", "g", isUnicodeTerminal(), "graphics chars (0=ASCII, 1=IBM-850, 2=VT100, 3=UTF-8)")
	// add [-A, --ascii, -G, --vt100, -U, --unicode]
	// add -C or --color to use colors
	// add -c --compact-not to turn line compaction on/off
	// things to change - start from the parent pid
	// maybe -h to high-light the current process in the tree

	if err := rootCmd.Execute(); err != nil {
		log.Errorf("Error: %v\n", err)
		//fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

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

func init() {

	log.Info("init()")

	config = Config{
		AOption:   false,
		MaxLDepth: 100,
		Graphics:  GraphicsASCII,
		TreeChar:  &treeChars[GraphicsASCII],
		SearchPid: -1,
		SearchStr: "",
	}

	myPID = os.Getpid()
	myPPID = os.Getppid()

}

func isUnicodeTerminal() int {
	// Check LANG and LC_CTYPE environment variables
	keys := []string{"LC_ALL", "LC_CTYPE", "LANG"}
	for _, key := range keys {
		val := os.Getenv(key)
		if strings.Contains(strings.ToUpper(val), "UTF-8") {
			// UTF
			return GraphicsUTF8
		}
	}
	// ASCII
	return GraphicsASCII
}

func getCurrentUsername() string {
	usr, err := user.Current()
	if err != nil {
		return ""
	}
	log.Infof("getCurrentUsername %s", usr.Username)
	return usr.Username
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

	if config.Long {
		return maxLine - 1
	}

	// Try to get terminal size

	// method 1 : term pkg
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
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
				proc.UID = int(sysStat.Uid)
				if u, err := user.LookupId(strconv.Itoa(int(proc.UID))); err == nil {
					proc.Owner = u.Username
				} else {
					proc.Owner = fmt.Sprintf("#%d", proc.UID)
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

		if pid, err := strconv.Atoi(statFields[0]); err == nil {
			proc.PID = pid
		} else {
			continue
		}

		proc.Cmd = strings.Trim(statFields[1], "()")

		if ppid, err := strconv.Atoi(statFields[3]); err == nil {
			proc.PPID = ppid
		}

		if pgid, err := strconv.Atoi(statFields[4]); err == nil {
			proc.PGID = pgid
		}

		proc.ThreadCount = 1

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

		proc.ParentIdx = -1
		proc.ChildIdx = -1
		proc.SisterIdx = -1
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

	// Use ps command based on OS
	var psCmd []string
	switch runtime.GOOS {
	case "linux":
		psCmd = []string{"ps", "-eo", "uid,pid,ppid,pgid,args"}
	case "darwin", "freebsd", "netbsd", "openbsd":
		psCmd = []string{"ps", "-axwwo", "user,pid,ppid,pgid,wq,comm"}
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
			if uid, err := strconv.Atoi(fields[0]); err == nil {
				proc.UID = uid
				if u, err := user.LookupId(fields[0]); err == nil {
					proc.Owner = u.Username
				} else {
					proc.Owner = fmt.Sprintf("#%s", fields[0])
				}
			}
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.Atoi(fields[2]); err == nil {
				proc.PPID = ppid
			}
			if pgid, err := strconv.Atoi(fields[3]); err == nil {
				proc.PGID = pgid
			}
			if len(fields) > 4 {
				if runtime.GOOS == "aix" && len(fields) > 5 {
					if thcount, err := strconv.Atoi(fields[4]); err == nil {
						proc.ThreadCount = thcount
					}
					proc.Cmd = strings.Join(fields[5:], " ")
				} else {
					proc.ThreadCount = 1
					proc.Cmd = strings.Join(fields[4:], " ")
				}
			}
		case "freebsd", "netbsd", "openbsd":
			proc.Owner = fields[0]
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.Atoi(fields[2]); err == nil {
				proc.PPID = ppid
			}
			if pgid, err := strconv.Atoi(fields[3]); err == nil {
				proc.PGID = pgid
			}
			if len(fields) > 4 {
				proc.Cmd = strings.Join(fields[4:], " ")
			}
			proc.ThreadCount = 1
		case "darwin":
			proc.Owner = fields[0]
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.Atoi(fields[2]); err == nil {
				proc.PPID = ppid
			}
			if pgid, err := strconv.Atoi(fields[3]); err == nil {
				proc.PGID = pgid
			}

			if len(fields) > 4 {

				if len(fields) > 5 {
					if thcount, err := strconv.Atoi(fields[4]); err == nil {
						proc.ThreadCount = thcount
					}
					proc.Cmd = fields[5]
				} else {
					proc.ThreadCount = 1
					proc.Cmd = fields[4]
				}

				proc.Cmd = stripPath(proc.Cmd)

			}
		default:
			// Default ps -ef format
			proc.Owner = fields[0]
			if pid, err := strconv.Atoi(fields[1]); err == nil {
				proc.PID = pid
			}
			if ppid, err := strconv.Atoi(fields[2]); err == nil {
				proc.PPID = ppid
			}
			if len(fields) > 7 {
				proc.Cmd = strings.Join(fields[7:], " ")
			}
			proc.ThreadCount = 1
		}

		proc.ParentIdx = -1
		proc.ChildIdx = -1
		proc.SisterIdx = -1
		proc.Print = false

		procs = append(procs, proc)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	nProc = len(procs)
	return nil
}

func stripPath(path string) string {

	//strip long paths
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash != -1 {
		return path[lastSlash+1:] // Everything after the last slash
	}
	return path
}
