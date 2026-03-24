package log

import (
	"flag"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kaiachain/kaia/log/term"
	"github.com/mattn/go-colorable"
)

// Enable logging to STDERR
// Exmaple use
//
//	log.EnableLogForTest(log.LvlCrit, log.LvlTrace)
//
// `normalLvl` is used in most cases
// `verboseLvl` is used if `go test -v` flag is given
func EnableLogForTest(normalLvl, verboseLvl Lvl) {
	lvl := Lvl(normalLvl)
	if isTestVerbose() {
		lvl = Lvl(verboseLvl)
	}

	usecolor := term.IsTty(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb"
	output := io.Writer(os.Stderr)
	if usecolor {
		output = colorable.NewColorableStderr()
	}

	glogger := NewGlogHandler(StreamHandler(output, TerminalFormat(usecolor)))
	PrintOrigins(true)
	ChangeGlobalLogLevel(glogger, lvl)
	glogger.Vmodule("")
	glogger.BacktraceAt("")
	Root().SetHandler(glogger)
}

func isTestVerbose() bool {
	// testing.Verbose panics before flags are parsed.
	if flag.Parsed() {
		return testing.Verbose()
	}
	return hasTruthyTestVerboseArg(os.Args[1:])
}

func hasTruthyTestVerboseArg(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "-test.v", arg == "--test.v", arg == "-v", arg == "--v":
			return true
		case strings.HasPrefix(arg, "-test.v="),
			strings.HasPrefix(arg, "--test.v="),
			strings.HasPrefix(arg, "-v="),
			strings.HasPrefix(arg, "--v="):
			if boolVal, ok := parseBoolFlagValue(arg); ok {
				return boolVal
			}
		}
	}
	return false
}

func parseBoolFlagValue(arg string) (bool, bool) {
	idx := strings.IndexByte(arg, '=')
	if idx < 0 || idx+1 >= len(arg) {
		return false, false
	}
	if val, err := strconv.ParseBool(arg[idx+1:]); err == nil {
		return val, true
	}
	return false, false
}
