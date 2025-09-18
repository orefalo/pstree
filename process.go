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

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/log"
)

var (

	// This holds the output of 'ps'
	procs []Process
	// number of discovered processes
	// TODO: why is this not procs.length
	nProc int

	// current rendering depth
	atLDepth int = 0
)

// printTree recursively prints the process tree
func printTree(idx int, head string) {
	if head == "" && !procs[idx].Print {
		return
	}

	if atLDepth == config.MaxLDepth {
		return
	}

	atLDepth++

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

	atLDepth--
}

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

	log.Errorf("pstree: No process found with PID == 1 || PPID == 0 || PPID == 1 || PID == PPID")
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

func stripPath(path string) string {

	//strip long paths
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash != -1 {
		return path[lastSlash+1:] // Everything after the last slash
	}
	return path
}

// getProcessesLinux reads processes directly from /proc filesystem (Linux)
func getProcessesLinux() error {
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

func debugPrintProcs(enforcePrintFlag bool) {
	if config.DOption {
		var (
			purple    = lipgloss.Color("99")
			gray      = lipgloss.Color("245")
			lightGray = lipgloss.Color("241")

			headerStyle  = lipgloss.NewStyle().Foreground(purple).Bold(true).Align(lipgloss.Center)
			cellStyle    = lipgloss.NewStyle().Padding(0, 1)
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
}
