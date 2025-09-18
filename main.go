package pstree

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
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
	// For long output (no width truncation)
	WOption bool
	// filter processes on this owner
	SearchOwner string
	// optional string to filter start processes
	SearchStr string
	// optional pid to start from, default parent pid
	SearchPid int

	// maximum tree depth
	MaxLDepth int

	// TODO: Compress output
	//Compress bool
	//Debug    bool
	// character set selector in treeChars
	Graphics int

	// terminal width in columns
	Columns int
	// character set used to render the tree
	TreeChar *TreeChars
}

var (
	// This holds the command line options
	config Config

	// that's mypid
	myPID int

	// and my parent pid
	myPPID int

	// what pid is the rendering starting from
	rootPID int
)

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

			return nil
		},
	}

	// Add flags
	rootCmd.Flags().StringVarP(&config.SearchOwner, "user", "u", getCurrentUsername(), "show only branches containing processes of user")
	rootCmd.Flags().BoolVarP(&config.UOption, "no-root", "U", false, "don't show branches containing only root processes")
	rootCmd.Flags().BoolVarP(&config.POption, "show-pids", "p", false, "show process pids")
	rootCmd.Flags().IntVarP(&config.MaxLDepth, "level", "l", 100, "print tree to n levels deep")
	rootCmd.Flags().BoolVarP(&config.AOption, "all", "a", false, "show all processes")
	rootCmd.Flags().BoolVarP(&config.WOption, "wide", "w", false, "wide output, not truncated to window width")
	rootCmd.Flags().BoolVarP(&config.DOption, "debug", "d", false, "print debugging info to stderr")
	rootCmd.Flags().IntVarP(&config.Graphics, "graphics", "g", isUnicodeTerminal(), "graphics chars (0=ASCII, 1=IBM-850, 2=VT100, 3=UTF-8)")
	// add [-A, --ascii, -G, --vt100, -U, --unicode]
	// add -C or --color to use colors
	// add -c --compact-not to turn line compaction on/off
	// things to change - start from the parent pid
	// maybe -h to high-light the current process in the tree

	if err := rootCmd.Execute(); err != nil {
		log.Errorf("Error: %v", err)
		os.Exit(1)
	}
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
