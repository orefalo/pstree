package main

const (
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
	// For wide output (no width truncation)
	WOption bool
	// filter processes on this owner
	SearchOwner string
	// optional string to filter start processes
	SearchStr string
	// optional pid to start from, default parent pid
	SearchPid int
	// maximum tree depth
	MaxLDepth int

	// character set selector in treeChars
	Graphics int
	// terminal width in columns
	Columns int
	// character set used to render the tree
	TreeChar *TreeChars
}
