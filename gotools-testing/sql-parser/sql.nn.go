package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)
import (
	"bufio"
	"io"
	"strings"
)

type frame struct {
	i            int
	s            string
	line, column int
}
type Lexer struct {
	// The lexer runs in its own goroutine, and communicates via channel 'ch'.
	ch      chan frame
	ch_stop chan bool
	// We record the level of nesting because the action could return, and a
	// subsequent call expects to pick up where it left off. In other words,
	// we're simulating a coroutine.
	// TODO: Support a channel-based variant that compatible with Go's yacc.
	stack []frame
	stale bool

	// The 'l' and 'c' fields were added for
	// https://github.com/wagerlabs/docker/blob/65694e801a7b80930961d70c69cba9f2465459be/buildfile.nex
	// Since then, I introduced the built-in Line() and Column() functions.
	l, c int

	parseResult interface{}

	// The following line makes it easy for scripts to insert fields in the
	// generated code.
	// [NEX_END_OF_LEXER_STRUCT]
}

// NewLexerWithInit creates a new Lexer object, runs the given callback on it,
// then returns it.
func NewLexerWithInit(in io.Reader, initFun func(*Lexer)) *Lexer {
	yylex := new(Lexer)
	if initFun != nil {
		initFun(yylex)
	}
	yylex.ch = make(chan frame)
	yylex.ch_stop = make(chan bool, 1)
	var scan func(in *bufio.Reader, ch chan frame, ch_stop chan bool, family []dfa, line, column int)
	scan = func(in *bufio.Reader, ch chan frame, ch_stop chan bool, family []dfa, line, column int) {
		// Index of DFA and length of highest-precedence match so far.
		matchi, matchn := 0, -1
		var buf []rune
		n := 0
		checkAccept := func(i int, st int) bool {
			// Higher precedence match? DFAs are run in parallel, so matchn is at most len(buf), hence we may omit the length equality check.
			if family[i].acc[st] && (matchn < n || matchi > i) {
				matchi, matchn = i, n
				return true
			}
			return false
		}
		var state [][2]int
		for i := 0; i < len(family); i++ {
			mark := make([]bool, len(family[i].startf))
			// Every DFA starts at state 0.
			st := 0
			for {
				state = append(state, [2]int{i, st})
				mark[st] = true
				// As we're at the start of input, follow all ^ transitions and append to our list of start states.
				st = family[i].startf[st]
				if -1 == st || mark[st] {
					break
				}
				// We only check for a match after at least one transition.
				checkAccept(i, st)
			}
		}
		atEOF := false
		stopped := false
		for {
			if n == len(buf) && !atEOF {
				r, _, err := in.ReadRune()
				switch err {
				case io.EOF:
					atEOF = true
				case nil:
					buf = append(buf, r)
				default:
					panic(err)
				}
			}
			if !atEOF {
				r := buf[n]
				n++
				var nextState [][2]int
				for _, x := range state {
					x[1] = family[x[0]].f[x[1]](r)
					if -1 == x[1] {
						continue
					}
					nextState = append(nextState, x)
					checkAccept(x[0], x[1])
				}
				state = nextState
			} else {
			dollar: // Handle $.
				for _, x := range state {
					mark := make([]bool, len(family[x[0]].endf))
					for {
						mark[x[1]] = true
						x[1] = family[x[0]].endf[x[1]]
						if -1 == x[1] || mark[x[1]] {
							break
						}
						if checkAccept(x[0], x[1]) {
							// Unlike before, we can break off the search. Now that we're at the end, there's no need to maintain the state of each DFA.
							break dollar
						}
					}
				}
				state = nil
			}

			if state == nil {
				lcUpdate := func(r rune) {
					if r == '\n' {
						line++
						column = 0
					} else {
						column++
					}
				}
				// All DFAs stuck. Return last match if it exists, otherwise advance by one rune and restart all DFAs.
				if matchn == -1 {
					if len(buf) == 0 { // This can only happen at the end of input.
						break
					}
					lcUpdate(buf[0])
					buf = buf[1:]
				} else {
					text := string(buf[:matchn])
					buf = buf[matchn:]
					matchn = -1
					for {
						sent := false
						select {
						case ch <- frame{matchi, text, line, column}:
							{
								sent = true
							}
						case stopped = <-ch_stop:
							{
							}
						default:
							{
								// nothing
							}
						}
						if stopped || sent {
							break
						}
					}
					if stopped {
						break
					}
					if len(family[matchi].nest) > 0 {
						scan(bufio.NewReader(strings.NewReader(text)), ch, ch_stop, family[matchi].nest, line, column)
					}
					if atEOF {
						break
					}
					for _, r := range text {
						lcUpdate(r)
					}
				}
				n = 0
				for i := 0; i < len(family); i++ {
					state = append(state, [2]int{i, 0})
				}
			}
		}
		ch <- frame{-1, "", line, column}
	}
	go scan(bufio.NewReader(in), yylex.ch, yylex.ch_stop, dfas, 0, 0)
	return yylex
}

type dfa struct {
	acc          []bool           // Accepting states.
	f            []func(rune) int // Transitions.
	startf, endf []int            // Transitions at start and end of input.
	nest         []dfa
}

var dfas = []dfa{
	// [Aa][Dd][Dd]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 68:
				return -1
			case 97:
				return 1
			case 100:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 2
			case 97:
				return -1
			case 100:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 3
			case 97:
				return -1
			case 100:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Ll][Ll]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 76:
				return -1
			case 97:
				return 1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return 2
			case 97:
				return -1
			case 108:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return 3
			case 97:
				return -1
			case 108:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 97:
				return -1
			case 108:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Ll][Tt][Ee][Rr]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 4
			case 76:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 4
			case 108:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Aa][Nn][Aa][Ll][Yy][Zz][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return 2
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return 2
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return 5
			case 90:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return 5
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 7
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return -1
			case 101:
				return 7
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 90:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			case 122:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Aa][Nn][Dd]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 68:
				return -1
			case 78:
				return -1
			case 97:
				return 1
			case 100:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 78:
				return 2
			case 97:
				return -1
			case 100:
				return -1
			case 110:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 3
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return 3
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Nn][Dd]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 68:
				return -1
			case 78:
				return -1
			case 97:
				return 1
			case 100:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 78:
				return 2
			case 97:
				return -1
			case 100:
				return -1
			case 110:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 3
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return 3
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Nn][Yy]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 78:
				return -1
			case 89:
				return -1
			case 97:
				return 1
			case 110:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 78:
				return 2
			case 89:
				return -1
			case 97:
				return -1
			case 110:
				return 2
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 78:
				return -1
			case 89:
				return 3
			case 97:
				return -1
			case 110:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 78:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 110:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Ss]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 83:
				return -1
			case 97:
				return 1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 83:
				return 2
			case 97:
				return -1
			case 115:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Aa][Ss][Cc]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 67:
				return -1
			case 83:
				return -1
			case 97:
				return 1
			case 99:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 83:
				return 2
			case 97:
				return -1
			case 99:
				return -1
			case 115:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Aa][Uu][Tt][Oo]_[Ii][Nn][Cc][Rr][Ee][Mm][Ee][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return 1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 5
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 7
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 7
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 8
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 8
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 9
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 9
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 10
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 10
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 11
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 11
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 12
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 12
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 13
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 13
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 14
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 14
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Bb][Ee][Ff][Oo][Rr][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 98:
				return 1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 2
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 98:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 70:
				return 3
			case 79:
				return -1
			case 82:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 102:
				return 3
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return 5
			case 98:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 6
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 98:
				return -1
			case 101:
				return 6
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Bb][Ee][Tt][Ww][Ee][Ee][Nn]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 69:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return 1
			case 101:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 2
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return 2
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 84:
				return 3
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 116:
				return 3
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return 4
			case 98:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 5
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return 5
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 6
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return 6
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 78:
				return 7
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 110:
				return 7
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt]8|[Bb][Ii][Gg][Ii][Nn][Tt]
	{[]bool{false, false, false, false, false, true, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return 1
			case 71:
				return -1
			case 73:
				return 2
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return 1
			case 103:
				return -1
			case 105:
				return 2
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return 6
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return 6
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 4
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return 5
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return 7
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return 7
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return 8
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return 8
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 9
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 9
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 10
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 10
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Bb][Ii][Nn][Aa][Rr][Yy]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return 2
			case 78:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return 2
			case 110:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return 4
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 89:
				return 6
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 121:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Bb][Ii][Tt]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 73:
				return -1
			case 84:
				return -1
			case 98:
				return 1
			case 105:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 73:
				return 2
			case 84:
				return -1
			case 98:
				return -1
			case 105:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 73:
				return -1
			case 84:
				return 3
			case 98:
				return -1
			case 105:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 73:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Bb][Ll][Oo][Bb]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 76:
				return -1
			case 79:
				return -1
			case 98:
				return 1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 76:
				return 2
			case 79:
				return -1
			case 98:
				return -1
			case 108:
				return 2
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 76:
				return -1
			case 79:
				return 3
			case 98:
				return -1
			case 108:
				return -1
			case 111:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 4
			case 76:
				return -1
			case 79:
				return -1
			case 98:
				return 4
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 98:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Bb][Oo][Tt][Hh]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 72:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 98:
				return 1
			case 104:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 72:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 98:
				return -1
			case 104:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 84:
				return 3
			case 98:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 72:
				return 4
			case 79:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 104:
				return 4
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 98:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Bb][Yy]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return 1
			case 89:
				return -1
			case 98:
				return 1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 89:
				return 2
			case 98:
				return -1
			case 121:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 89:
				return -1
			case 98:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Cc][Aa][Ll][Ll]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 76:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 76:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 76:
				return 3
			case 97:
				return -1
			case 99:
				return -1
			case 108:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 76:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 108:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 76:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 108:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Cc][Aa][Ss][Cc][Aa][Dd][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 3
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return 5
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 6
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 6
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 7
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Aa][Ss][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 83:
				return 3
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 4
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Cc][Hh][Aa][Nn][Gg][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 72:
				return 2
			case 78:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 104:
				return 2
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 67:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 97:
				return 3
			case 99:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 78:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 110:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 71:
				return 5
			case 72:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 103:
				return 5
			case 104:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 6
			case 71:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 6
			case 103:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Hh][Aa][Rr]([Aa][Cc][Tt][Ee][Rr])?
	{[]bool{false, false, false, false, true, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 3
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 5
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 6
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 6
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return 7
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 8
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 8
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 9
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 9
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Hh][Ee][Cc][Kk]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 72:
				return -1
			case 75:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 107:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 2
			case 75:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 107:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 3
			case 72:
				return -1
			case 75:
				return -1
			case 99:
				return -1
			case 101:
				return 3
			case 104:
				return -1
			case 107:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 69:
				return -1
			case 72:
				return -1
			case 75:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 104:
				return -1
			case 107:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 75:
				return 5
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 107:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 75:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 107:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Ll][Ll][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return 5
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 7
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 7
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Ll][Uu][Mm][Nn]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return 1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 85:
				return -1
			case 99:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return 3
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 108:
				return 3
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return 4
			case 99:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return -1
			case 77:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 108:
				return -1
			case 109:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Mm][Mm][Ee][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return 4
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return 4
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 5
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 7
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Nn][Dd][Ii][Tt][Ii][Oo][Nn]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return 1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 4
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 6
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 8
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 8
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return 9
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return 9
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Nn][Ss][Tt][Rr][Aa][Ii][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 6
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 6
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 7
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return 8
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return 8
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return 9
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return 9
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 10
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 10
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Nn][Tt][Ii][Nn][Uu][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 4
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 7
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Nn][Vv][Ee][Rr][Tt]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 6
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 6
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 7
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 7
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Rr][Ee][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 3
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 3
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 67:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Rr][Oo][Ss][Ss]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return 2
			case 83:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return 3
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 111:
				return 3
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 4
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 5
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Uu][Rr][Rr][Ee][Nn][Tt]_[Dd][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 8
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 9
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 9
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 10
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 10
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 11
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 11
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 12
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 12
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Uu][Rr][Rr][Ee][Nn][Tt]_[Tt][Ii][Mm][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 5
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 8
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 9
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 9
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 10
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 10
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 11
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 11
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 12
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return 12
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Uu][Rr][Rr][Ee][Nn][Tt]_[Tt][Ii][Mm][Ee][Ss][Tt][Aa][Mm][Pp]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return 4
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return 4
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 5
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 6
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 6
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 8
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 9
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 9
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 10
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 10
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 11
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 11
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 12
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 12
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return 13
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return 13
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 14
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 14
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 15
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 15
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 16
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 16
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return 17
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return 17
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Uu][Rr][Rr][Ee][Nn][Tt]_[Uu][Ss][Ee][Rr]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 4
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 4
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 8
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 9
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 10
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 10
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 11
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return 11
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 12
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 12
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Uu][Rr][Ss][Oo][Rr]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 99:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return 2
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 83:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 4
			case 85:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return 5
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 111:
				return 5
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return 6
			case 83:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return 6
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Aa][Bb][Aa][Ss][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 5
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return 5
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 6
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 7
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 7
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return 8
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Aa][Bb][Aa][Ss][Ee][Ss]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 5
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return 5
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 6
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 7
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 7
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return 8
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 9
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 9
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Ee][Tt][Ii][Mm][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 4
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 7
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 7
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Yy]_[Hh][Oo][Uu][Rr]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return 3
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return 5
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return 5
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return 6
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return 6
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return 7
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return 7
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return 8
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return 8
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 72:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Yy]_[Mm][Ii][Cc][Rr][Oo][Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return 3
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 7
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 7
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 8
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 8
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 9
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 9
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 10
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 10
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 11
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 11
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 12
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 12
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 13
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 13
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 14
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 14
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 15
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 15
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Yy]_[Mm][Ii][Nn][Uu][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return 3
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 5
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 5
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 7
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 7
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return 8
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return 8
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return 9
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return 9
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 10
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 10
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Yy]_[Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return 3
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return 5
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return 5
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 7
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 7
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return 8
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return 8
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return 9
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return 9
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 10
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 10
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Nn][Uu][Mm][Ee][Rr][Ii][Cc]|[Dd][Ee][Cc]|[Dd][Ee][Cc][Ii][Mm][Aa][Ll]
	{[]bool{false, false, false, false, false, false, false, false, true, false, true, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 2
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 2
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 9
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return 3
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 4
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 4
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 5
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 5
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 6
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 6
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 8
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return 8
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 11
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 11
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 12
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 12
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 13
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return 13
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 14
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 14
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Cc][Ll][Aa][Rr][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return 5
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return 6
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 7
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Ff][Aa][Uu][Ll][Tt]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return 3
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return 3
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return 5
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 6
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 6
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Ll][Aa][Yy][Ee][Dd]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 89:
				return 5
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 121:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 7
			case 69:
				return -1
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return 7
			case 101:
				return -1
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Ll][Ee][Tt][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return 1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 84:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 84:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 4
			case 76:
				return -1
			case 84:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return 5
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 6
			case 76:
				return -1
			case 84:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Ss][Cc]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 83:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 3
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 99:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Ss][Cc][Rr][Ii][Bb][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return 3
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 7
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return 7
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 98:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ee][Tt][Ee][Rr][Mm][Ii][Nn][Ii][Ss][Tt][Ii][Cc]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 4
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 6
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 6
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 8
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 8
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 9
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 9
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 10
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 10
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 11
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 12
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 12
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 13
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return 13
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ii][Ss][Tt][Ii][Nn][Cc][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 2
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 2
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 3
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 4
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 7
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return 7
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 8
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ii][Ss][Tt][Ii][Nn][Cc][Tt][Rr][Oo][Ww]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 2
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 2
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 3
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 3
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 4
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 4
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 7
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return 7
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 8
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 8
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 9
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 9
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 10
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 10
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return 11
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Ii][Vv]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return 1
			case 73:
				return -1
			case 86:
				return -1
			case 100:
				return 1
			case 105:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 73:
				return 2
			case 86:
				return -1
			case 100:
				return -1
			case 105:
				return 2
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 73:
				return -1
			case 86:
				return 3
			case 100:
				return -1
			case 105:
				return -1
			case 118:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 73:
				return -1
			case 86:
				return -1
			case 100:
				return -1
			case 105:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ff][Ll][Oo][Aa][Tt]8|[Dd][Oo][Uu][Bb][Ll][Ee]
	{[]bool{false, false, false, false, false, false, false, true, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 70:
				return 2
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 102:
				return 2
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return 8
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return 8
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 3
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 3
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return 5
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 5
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return 6
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return 6
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return 7
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 9
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 11
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 11
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return 12
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return 12
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 56:
				return -1
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Rr][Oo][Pp]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return 1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 100:
				return 1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 2
			case 100:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 79:
				return 3
			case 80:
				return -1
			case 82:
				return -1
			case 100:
				return -1
			case 111:
				return 3
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 79:
				return -1
			case 80:
				return 4
			case 82:
				return -1
			case 100:
				return -1
			case 111:
				return -1
			case 112:
				return 4
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 100:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Dd][Uu][Aa][Ll]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 76:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 108:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return -1
			case 85:
				return 2
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 68:
				return -1
			case 76:
				return -1
			case 85:
				return -1
			case 97:
				return 3
			case 100:
				return -1
			case 108:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return 4
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ee][Aa][Cc][Hh]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 1
			case 72:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 1
			case 104:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 69:
				return -1
			case 72:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 101:
				return -1
			case 104:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ee][Ll][Ss][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return 1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return 1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 76:
				return 2
			case 83:
				return -1
			case 101:
				return -1
			case 108:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return 3
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return 4
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ee][Ll][Ss][Ee][Ii][Ff]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return 1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return 1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return 2
			case 83:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 83:
				return 3
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 115:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return 4
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 5
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 5
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 6
			case 73:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 102:
				return 6
			case 105:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ee][Nn][Dd]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 1
			case 78:
				return -1
			case 100:
				return -1
			case 101:
				return 1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 69:
				return -1
			case 78:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 78:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ee][Nn][Uu][Mm]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return 1
			case 77:
				return -1
			case 78:
				return -1
			case 85:
				return -1
			case 101:
				return 1
			case 109:
				return -1
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return 2
			case 85:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 85:
				return 3
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return 4
			case 78:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 109:
				return 4
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ee][Ss][Cc][Aa][Pp][Ee][Dd]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 1
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 83:
				return 2
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 115:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return 4
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return 5
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return 5
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 7
			case 69:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 7
			case 101:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ee][Xx][Ii][Ss][Tt][Ss]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return 1
			case 73:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return 1
			case 105:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return 2
			case 101:
				return -1
			case 105:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 83:
				return 4
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 115:
				return 4
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 83:
				return 6
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 115:
				return 6
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Nn][Oo][Tt][ \t\n]+[Ee][Xx][Ii][Ss][Tt][Ss]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 4
			case 10:
				return 4
			case 32:
				return 4
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 4
			case 10:
				return 4
			case 32:
				return 4
			case 69:
				return 5
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return 5
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return 6
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return 8
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return 8
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return 9
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return 9
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return 10
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return 10
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ee][Xx][Ii][Tt]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return 1
			case 73:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return 1
			case 105:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 84:
				return -1
			case 88:
				return 2
			case 101:
				return -1
			case 105:
				return -1
			case 116:
				return -1
			case 120:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 3
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return 3
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 84:
				return 4
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 116:
				return 4
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ee][Xx][Pp][Ll][Aa][Ii][Nn]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return 1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return 2
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return 3
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return 3
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 4
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 4
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 7
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 7
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 80:
				return -1
			case 88:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 112:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Ee][Tt][Cc][Hh]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return 1
			case 72:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return 1
			case 104:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 2
			case 70:
				return -1
			case 72:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 104:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 72:
				return -1
			case 84:
				return 3
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 104:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 69:
				return -1
			case 70:
				return -1
			case 72:
				return -1
			case 84:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 102:
				return -1
			case 104:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 72:
				return 5
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 104:
				return 5
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 72:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 104:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Ll][Oo][Aa][Tt]4?
	{[]bool{false, false, false, false, false, true, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return -1
			case 70:
				return 1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 102:
				return 1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return -1
			case 70:
				return -1
			case 76:
				return 2
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 102:
				return -1
			case 108:
				return 2
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return 3
			case 84:
				return -1
			case 97:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return 4
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return 6
			case 65:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 65:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Oo][Rr]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 70:
				return 1
			case 79:
				return -1
			case 82:
				return -1
			case 102:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 102:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ff][Oo][Rr][Cc][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return 1
			case 79:
				return -1
			case 82:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 5
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 99:
				return -1
			case 101:
				return 5
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Oo][Rr][Ee][Ii][Gg][Nn]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return 1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return 4
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return 6
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return 6
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 7
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 7
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Rr][Oo][Mm]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 70:
				return 1
			case 77:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 102:
				return 1
			case 109:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 82:
				return 2
			case 102:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 114:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 77:
				return -1
			case 79:
				return 3
			case 82:
				return -1
			case 102:
				return -1
			case 109:
				return -1
			case 111:
				return 3
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 77:
				return 4
			case 79:
				return -1
			case 82:
				return -1
			case 102:
				return -1
			case 109:
				return 4
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 102:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ff][Uu][Ll][Ll][Tt][Ee][Xx][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return 1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 3
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 3
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return 6
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return 7
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return 8
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return 8
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Gg][Rr][Aa][Nn][Tt]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return 1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 78:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 110:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 71:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 3
			case 103:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 78:
				return 4
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 110:
				return 4
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 103:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Gg][Rr][Oo][Uu][Pp]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 71:
				return 1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 103:
				return 1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 2
			case 85:
				return -1
			case 103:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 79:
				return 3
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 111:
				return 3
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return 4
			case 103:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 79:
				return -1
			case 80:
				return 5
			case 82:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 111:
				return -1
			case 112:
				return 5
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Hh][Aa][Vv][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return 1
			case 73:
				return -1
			case 78:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 86:
				return -1
			case 97:
				return 2
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 86:
				return 3
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 118:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 4
			case 78:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 4
			case 110:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 78:
				return 5
			case 86:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 110:
				return 5
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 6
			case 72:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 103:
				return 6
			case 104:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Hh][Ii][Gg][Hh]_[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Yy]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return 1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return 1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 2
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 2
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return 3
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return 3
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return 4
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return 4
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return 5
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 7
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 7
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 8
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 8
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return 9
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return 9
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 10
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 10
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 11
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 11
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return 12
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return 12
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return 13
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return 13
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Hh][Oo][Uu][Rr]_[Mm][Ii][Cc][Rr][Oo][Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return 1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return 1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return 3
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 4
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 4
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return 5
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return 6
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return 6
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return 7
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return 7
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 8
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 8
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 9
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 9
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 10
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 10
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 11
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 11
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 12
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 12
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 13
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 13
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 14
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 14
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 15
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 15
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 16
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 16
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Hh][Oo][Uu][Rr]_[Mm][Ii][Nn][Uu][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return 1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return 1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 3
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 5
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return 6
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return 6
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return 7
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return 7
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 8
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 8
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 9
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 10
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 10
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 11
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return 11
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Hh][Oo][Uu][Rr]_[Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return 3
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 4
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 4
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return 5
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 6
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 6
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 7
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 8
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 8
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return 9
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return 9
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return 10
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return 10
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 11
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 11
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Ff]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 73:
				return 1
			case 102:
				return -1
			case 105:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return 2
			case 73:
				return -1
			case 102:
				return 2
			case 105:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 70:
				return -1
			case 73:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Ii][Gg][Nn][Oo][Rr][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 2
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 103:
				return 2
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 5
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return 6
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return 1
			case 78:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 2
			case 105:
				return -1
			case 110:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Ii][Nn][Ff][Ii][Ll][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 1
			case 76:
				return -1
			case 78:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 2
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 3
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 101:
				return -1
			case 102:
				return 3
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return 5
			case 78:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return 5
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 101:
				return 6
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Nn][Ee][Rr]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 82:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 82:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Oo][Uu][Tt]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 3
			case 84:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 3
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Ss][Ee][Nn][Ss][Ii][Tt][Ii][Vv][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 3
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 3
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 5
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 5
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 6
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 6
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 8
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 8
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 9
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return 9
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return 10
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return 10
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 11
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return 11
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Ss][Ee][Rr][Tt]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 3
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 6
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt]4?|[Ii][Nn][Tt][Ee][Gg][Ee][Rr]
	{[]bool{false, false, false, true, true, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return 4
			case 69:
				return 5
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return 5
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return 6
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return 6
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return 7
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return 7
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 8
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 8
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 52:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt][Ee][Rr][Vv][Aa][Ll]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return 1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 4
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return 7
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 8
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 8
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt][Oo]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 3
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ii][Ss]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return 1
			case 83:
				return -1
			case 105:
				return 1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 83:
				return 2
			case 105:
				return -1
			case 115:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 83:
				return -1
			case 105:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Ii][Tt][Ee][Rr][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return 1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return 2
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 3
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 3
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 7
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 7
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Jj][Oo][Ii][Nn]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 74:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 105:
				return -1
			case 106:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 3
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 105:
				return 3
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return 4
			case 79:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return 4
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Dd][Ee][Xx]|[Kk][Ee][Yy]
	{[]bool{false, false, false, false, true, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 1
			case 75:
				return 2
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 1
			case 107:
				return 2
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return 5
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return 5
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 3
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return 3
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 6
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return 6
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 7
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return 8
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return 8
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 78:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Kk][Ee][Yy][Ss]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return 1
			case 83:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 107:
				return 1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 75:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 101:
				return 2
			case 107:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 83:
				return -1
			case 89:
				return 3
			case 101:
				return -1
			case 107:
				return -1
			case 115:
				return -1
			case 121:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 83:
				return 4
			case 89:
				return -1
			case 101:
				return -1
			case 107:
				return -1
			case 115:
				return 4
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 83:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 107:
				return -1
			case 115:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Kk][Ii][Ll][Ll]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 75:
				return 1
			case 76:
				return -1
			case 105:
				return -1
			case 107:
				return 1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 2
			case 75:
				return -1
			case 76:
				return -1
			case 105:
				return 2
			case 107:
				return -1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return 3
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return 4
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Ee][Aa][Dd][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 4
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return 4
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 5
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 5
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 6
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return 7
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return 7
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Ee][Aa][Vv][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 76:
				return -1
			case 86:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 108:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 86:
				return 4
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 118:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 5
			case 76:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return 5
			case 108:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Ee][Ff][Tt]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 1
			case 84:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 3
			case 76:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 102:
				return 3
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return 4
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Ii][Kk][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 2
			case 75:
				return -1
			case 76:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 107:
				return -1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return 3
			case 76:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return 3
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Ii][Mm][Ii][Tt]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return 1
			case 77:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 2
			case 76:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 105:
				return 2
			case 108:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 3
			case 84:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 4
			case 76:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 105:
				return 4
			case 108:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 84:
				return 5
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Ii][Nn][Ee][Ss]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 1
			case 78:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 110:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 2
			case 76:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 108:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 3
			case 83:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 3
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 83:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 115:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Aa][Dd]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return 1
			case 79:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return 1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return -1
			case 79:
				return 2
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 68:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 97:
				return 3
			case 100:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 4
			case 76:
				return -1
			case 79:
				return -1
			case 97:
				return -1
			case 100:
				return 4
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Cc][Aa][Ll][Tt][Ii][Mm][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 5
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 5
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 8
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 8
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 9
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Cc][Aa][Ll][Tt][Ii][Mm][Ee][Ss][Tt][Aa][Mm][Pp]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return 2
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return 2
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 5
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 5
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 8
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 8
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 9
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return 10
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return 10
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 11
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 12
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 12
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 13
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 13
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return 14
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return 14
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Cc][Kk]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return 1
			case 79:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return 1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 79:
				return 2
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 3
			case 75:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 99:
				return 3
			case 107:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return 4
			case 76:
				return -1
			case 79:
				return -1
			case 99:
				return -1
			case 107:
				return 4
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Nn][Gg]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 76:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 103:
				return -1
			case 108:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 103:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Nn][Gg][Bb][Ll][Oo][Bb]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return -1
			case 103:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 5
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return 5
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return 6
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return 6
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 7
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 8
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return 8
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Nn][Gg][Tt][Ee][Xx][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 5
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 5
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return 6
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return 7
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 8
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 8
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Oo][Pp]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 76:
				return 1
			case 79:
				return -1
			case 80:
				return -1
			case 108:
				return 1
			case 111:
				return -1
			case 112:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return 2
			case 80:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			case 112:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return 3
			case 80:
				return -1
			case 108:
				return -1
			case 111:
				return 3
			case 112:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return 4
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ll][Oo][Ww]_[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Yy]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return 1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return 1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return 2
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return 3
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return 3
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return 5
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return 5
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 6
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 6
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 7
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return 7
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return 8
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return 8
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 9
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 9
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 10
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return 10
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return 11
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return 11
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return 12
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return 12
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Aa][Tt][Cc][Hh]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 72:
				return -1
			case 77:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 104:
				return -1
			case 109:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 84:
				return 3
			case 97:
				return -1
			case 99:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 4
			case 72:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return 4
			case 104:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 72:
				return 5
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 104:
				return 5
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Ee][Dd][Ii][Uu][Mm][Bb][Ll][Oo][Bb]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 4
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 4
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return 5
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 6
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 6
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 7
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return 7
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 8
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 8
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return 9
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return 9
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Ii][Dd][Dd][Ll][Ee][Ii][Nn][Tt]|[Mm][Ee][Dd][Ii][Uu][Mm][Ii][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return 3
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return 3
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 11
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return 11
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 4
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return 4
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 5
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 7
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 8
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 8
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 9
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 9
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return 10
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return 10
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 12
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 12
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return 13
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return 13
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 14
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 14
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 15
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 15
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 16
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 16
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return 17
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return 17
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Ee][Dd][Ii][Uu][Mm][Tt][Ee][Xx][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 4
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 4
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return 5
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return 5
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 6
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 6
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return 9
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 10
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 10
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 88:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Ii][Nn][Uu][Tt][Ee]_[Mm][Ii][Cc][Rr][Oo][Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 2
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 4
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 7
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 8
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 8
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 9
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 9
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 11
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 11
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 12
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 12
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 13
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 13
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 14
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 14
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 15
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 15
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 16
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 16
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 17
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 17
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 18
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 18
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Ii][Nn][Uu][Tt][Ee]_[Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 2
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 4
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 7
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return 8
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return 8
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 9
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 11
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 11
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 12
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 12
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 13
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 13
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Mm][Oo][Dd]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 77:
				return 1
			case 79:
				return -1
			case 100:
				return -1
			case 109:
				return 1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 77:
				return -1
			case 79:
				return 2
			case 100:
				return -1
			case 109:
				return -1
			case 111:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 77:
				return -1
			case 79:
				return -1
			case 100:
				return 3
			case 109:
				return -1
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 100:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Mm][Oo][Dd][Ii][Ff][Ii][Ee][Ss]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return 1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return 1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return 2
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 4
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 4
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return 5
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return 5
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 7
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return 8
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Nn][Aa][Tt][Uu][Rr][Aa][Ll]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 78:
				return 1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 108:
				return -1
			case 110:
				return 1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 2
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 97:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 4
			case 97:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 6
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return 7
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 108:
				return 7
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Nn][Oo][Tt]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 78:
				return 1
			case 79:
				return -1
			case 84:
				return -1
			case 110:
				return 1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Nn][Oo]_[Ww][Rr][Ii][Tt][Ee]_[Tt][Oo]_[Bb][Ii][Nn][Ll][Oo][Gg]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return 3
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return 4
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 6
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 6
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 7
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 7
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 8
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return 8
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return 9
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 10
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 10
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 11
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 11
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return 12
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 13
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return 13
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 14
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 14
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 15
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 15
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 16
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 16
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 17
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 17
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return 18
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return 18
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Nn][Uu][Ll][Ll]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 78:
				return 1
			case 85:
				return -1
			case 108:
				return -1
			case 110:
				return 1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 78:
				return -1
			case 85:
				return 2
			case 108:
				return -1
			case 110:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 3
			case 78:
				return -1
			case 85:
				return -1
			case 108:
				return 3
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 4
			case 78:
				return -1
			case 85:
				return -1
			case 108:
				return 4
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 78:
				return -1
			case 85:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Nn][Uu][Mm][Bb][Ee][Rr]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return 1
			case 82:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return 1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return 2
			case 98:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 77:
				return 3
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 109:
				return 3
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 4
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 98:
				return 4
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 5
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 101:
				return 5
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 6
			case 85:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 6
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Nn]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 78:
				return -1
			case 79:
				return 1
			case 110:
				return -1
			case 111:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 78:
				return 2
			case 79:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 78:
				return -1
			case 79:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Oo][Nn][ \t\n]+[Dd][Uu][Pp][Ll][Ii][Cc][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 3
			case 10:
				return 3
			case 32:
				return 3
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 3
			case 10:
				return 3
			case 32:
				return 3
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 4
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 4
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return 5
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return 6
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return 6
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 7
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 7
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 8
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 8
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return 9
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return 9
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return 10
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 10
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return 11
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return 11
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 12
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 12
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Pp][Tt][Ii][Mm][Ii][Zz][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return 1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return 1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return 2
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return 2
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return 3
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return 3
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 4
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return 4
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 5
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 5
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return 7
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 8
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 122:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Pp][Tt][Ii][Oo][Nn]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 1
			case 80:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 1
			case 112:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return 2
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return 3
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 4
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 105:
				return 4
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 5
			case 80:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 5
			case 112:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Pp][Tt][Ii][Oo][Nn][Aa][Ll][Ll][Yy]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return 2
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return 2
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return 3
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return 3
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 5
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 5
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 6
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 6
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return 7
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return 8
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return 8
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return 9
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return 9
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return 10
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return 10
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Rr]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 79:
				return 1
			case 82:
				return -1
			case 111:
				return 1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 82:
				return 2
			case 111:
				return -1
			case 114:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 82:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Oo][Rr][Dd][Ee][Rr]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return 1
			case 82:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return 1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 4
			case 79:
				return -1
			case 82:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return 5
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Uu][Tt]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 79:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 111:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 111:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Oo][Uu][Tt][Ee][Rr]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 79:
				return 1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 111:
				return 1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return 4
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Oo][Uu][Tt][Ff][Ii][Ll][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 4
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 5
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 5
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return 6
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return 6
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 7
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return 7
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Pp][Rr][Ee][Cc][Ii][Ss][Ii][Oo][Nn]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return 1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return 1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 2
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 3
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 3
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return 6
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 8
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 8
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 9
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 9
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Pp][Rr][Ii][Mm][Aa][Rr][Yy]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return 1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return 1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return 2
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return 2
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return 3
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return 3
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return 4
			case 80:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return 4
			case 112:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return 5
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return 6
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return 6
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 89:
				return 7
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 121:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Pp][Rr][Oo][Cc][Ee][Dd][Uu][Rr][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return 1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return 1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 2
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return 3
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return 3
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 5
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 5
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 6
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return 6
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return 7
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 8
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 8
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 9
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 9
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Pp][Uu][Rr][Gg][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 80:
				return 1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 112:
				return 1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return 2
			case 101:
				return -1
			case 103:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 80:
				return -1
			case 82:
				return 3
			case 85:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 112:
				return -1
			case 114:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 4
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 103:
				return 4
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 5
			case 71:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return 5
			case 103:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Qq][Uu][Ii][Cc][Kk]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 81:
				return 1
			case 85:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 113:
				return 1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 81:
				return -1
			case 85:
				return 2
			case 99:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 113:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 73:
				return 3
			case 75:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 105:
				return 3
			case 107:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 73:
				return -1
			case 75:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 99:
				return 4
			case 105:
				return -1
			case 107:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 73:
				return -1
			case 75:
				return 5
			case 81:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 107:
				return 5
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Aa][Dd]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return 1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 82:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 97:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 4
			case 69:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 100:
				return 4
			case 101:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Aa][Dd][Ss]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return 1
			case 83:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return 1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 4
			case 69:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 100:
				return 4
			case 101:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 83:
				return 5
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 115:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Aa][Ll]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Ff][Ee][Rr][Ee][Nn][Cc][Ee][Ss]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return 1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return 1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 2
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return 3
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return 3
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 4
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 6
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 6
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return 7
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return 7
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 8
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return 8
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 9
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 9
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 10
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 10
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Gg][Ee][Xx][Pp]|[Rr][Ll][Ii][Kk][Ee]
	{[]bool{false, false, false, false, false, false, true, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return 1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return 1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return 3
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return 2
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return 3
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 7
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return 7
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 4
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 4
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return 5
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return 5
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return 6
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 8
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return 8
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return 9
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return 10
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return 10
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Ll][Ee][Aa][Ss][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return 1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return 1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 4
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return 4
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return 5
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 7
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return 7
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Nn][Aa][Mm][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return 3
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return 3
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 97:
				return 4
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Pp][Ee][Aa][Tt]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 82:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 114:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 80:
				return 3
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 112:
				return 3
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 4
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 4
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 69:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 5
			case 101:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Pp][Ll][Aa][Cc][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return 1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 80:
				return 3
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 112:
				return 3
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return 5
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 6
			case 69:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return 6
			case 101:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 7
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 7
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Qq][Uu][Ii][Rr][Ee]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return 1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return 1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 81:
				return 3
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 113:
				return 3
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 85:
				return 4
			case 101:
				return -1
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 5
			case 81:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return 5
			case 113:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return 6
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return 6
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 7
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return 7
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Ss][Tt][Rr][Ii][Cc][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return 1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return 1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return 3
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 7
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return 7
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 8
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Tt][Uu][Rr][Nn]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return 2
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 4
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return 5
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return 5
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ee][Vv][Oo][Kk][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 79:
				return -1
			case 82:
				return 1
			case 86:
				return -1
			case 101:
				return -1
			case 107:
				return -1
			case 111:
				return -1
			case 114:
				return 1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 75:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 101:
				return 2
			case 107:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 86:
				return 3
			case 101:
				return -1
			case 107:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 118:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 107:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return 5
			case 79:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 107:
				return 5
			case 111:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 75:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 101:
				return 6
			case 107:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 75:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 107:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Ii][Gg][Hh][Tt]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 82:
				return 1
			case 84:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 114:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 2
			case 82:
				return -1
			case 84:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 2
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return 3
			case 72:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 103:
				return 3
			case 104:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return 4
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 103:
				return -1
			case 104:
				return 4
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return 5
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Rr][Oo][Ll][Ll][Uu][Pp]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 1
			case 85:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return 2
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 108:
				return -1
			case 111:
				return 2
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 3
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 108:
				return 3
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 4
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 108:
				return 4
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return 5
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return 6
			case 82:
				return -1
			case 85:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return 6
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 85:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Cc][Hh][Ee][Mm][Aa]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return 1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 2
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 2
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 3
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 3
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 4
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return 5
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return 5
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return 6
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Cc][Hh][Ee][Mm][Aa][Ss]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return 1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 2
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return 2
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 3
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 3
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 4
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return 5
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return 5
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return 6
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return 7
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ee][Cc][Oo][Nn][Dd]_[Mm][Ii][Cc][Rr][Oo][Ss][Ee][Cc][Oo][Nn][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 5
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 5
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 6
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 6
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return 7
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 8
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 8
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 9
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 9
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 10
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return 10
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 11
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 11
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 12
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 12
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 13
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 13
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 14
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 14
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 15
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return 15
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 16
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 16
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 17
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 17
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return 18
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return 18
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ee][Ll][Ee][Cc][Tt]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 2
			case 76:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return 2
			case 108:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 4
			case 76:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 108:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 5
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return 5
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 84:
				return 6
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ee][Nn][Ss][Ii][Tt][Ii][Vv][Ee]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 4
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 4
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 6
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 6
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return 8
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 9
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ee][Pp][Aa][Rr][Aa][Tt][Oo][Rr]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return 3
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return 3
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 5
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 5
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 6
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 7
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return 8
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return 8
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 9
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 9
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ee][Tt]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 101:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ss][Hh][Oo][Ww]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 79:
				return -1
			case 83:
				return 1
			case 87:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 115:
				return 1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return 2
			case 79:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 104:
				return 2
			case 111:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 79:
				return 3
			case 83:
				return -1
			case 87:
				return -1
			case 104:
				return -1
			case 111:
				return 3
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 87:
				return 4
			case 104:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 119:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 104:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt]2|[Ss][Mm][Aa][Ll][Ll][Ii][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, true, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return 1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return 1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 10
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 10
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return 3
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return 3
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return 4
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return 5
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return 5
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return 7
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return 7
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return 8
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return 8
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 9
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return 11
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return 12
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 50:
				return -1
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Oo][Mm][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return 1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return 2
			case 83:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return 3
			case 79:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 109:
				return 3
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 101:
				return 4
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ss][Oo][Nn][Aa][Mm][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return 3
			case 79:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return 3
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 97:
				return 4
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return 5
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return 5
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 6
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return 6
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Pp][Aa][Tt][Ii][Aa][Ll]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 3
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 4
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return 5
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return 5
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 6
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return 7
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return 7
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Pp][Ee][Cc][Ii][Ff][Ii][Cc]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return 1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return 2
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 3
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return 3
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 4
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 5
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 5
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return 6
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return 6
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 7
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 7
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 8
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return 8
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return 1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 81:
				return 2
			case 83:
				return -1
			case 108:
				return -1
			case 113:
				return 2
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 3
			case 81:
				return -1
			case 83:
				return -1
			case 108:
				return 3
			case 113:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll][Ee][Xx][Cc][Ee][Pp][Tt][Ii][Oo][Nn]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 4
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return 5
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 6
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return 6
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 7
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return 7
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return 8
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return 8
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return 9
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return 9
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 10
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 10
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 11
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 11
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 12
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 12
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll][Ss][Tt][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return 2
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return 2
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 6
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return 7
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 8
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 8
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll][Ww][Aa][Rr][Nn][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 3
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 3
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return 4
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 5
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return 5
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return 6
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return 6
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 7
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 7
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return 8
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return 8
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 9
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 9
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 10
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return 10
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 87:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll]_[Bb][Ii][Gg]_[Rr][Ee][Ss][Uu][Ll][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 3
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 3
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 4
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 5
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return 5
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 6
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 6
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return 7
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return 7
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 8
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return 9
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return 9
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return 10
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return 10
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 11
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 11
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 12
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 12
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 13
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 13
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 14
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 14
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll]_[Cc][Aa][Ll][Cc]_[Ff][Oo][Uu][Nn][Dd]_[Rr][Oo][Ww][Ss]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 5
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 5
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return 6
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return 7
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return 7
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 8
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 8
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return 9
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return 10
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return 10
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 11
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 11
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return 12
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return 12
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return 13
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return 13
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 14
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 14
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return 15
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return 16
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return 16
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 17
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 17
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return 18
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return 18
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 19
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 19
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Qq][Ll]_[Ss][Mm][Aa][Ll][Ll]_[Rr][Ee][Ss][Uu][Ll][Tt]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return 2
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return 2
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 5
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 5
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return 6
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return 6
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 7
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 8
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 8
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 9
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 9
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 10
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return 11
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return 11
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 12
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return 12
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return 13
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return 13
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 14
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 14
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 15
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 15
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 16
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 16
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 77:
				return -1
			case 81:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 109:
				return -1
			case 113:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Ss][Ll]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 83:
				return 1
			case 108:
				return -1
			case 115:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 83:
				return 2
			case 108:
				return -1
			case 115:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return 3
			case 83:
				return -1
			case 108:
				return 3
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 76:
				return -1
			case 83:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Ss][Tt][Aa][Rr][Tt][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 2
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 3
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 4
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 4
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return 6
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return 6
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 7
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 7
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 8
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return 8
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ss][Tt][Rr][Aa][Ii][Gg][Hh][Tt]_[Jj][Oo][Ii][Nn]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 2
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return 4
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 5
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 5
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 6
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return 6
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return 7
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return 7
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 8
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return 9
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return 10
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return 10
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return 11
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return 11
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return 12
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return 12
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return 13
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return 13
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 74:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 106:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Aa][Bb][Ll][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return 1
			case 97:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 66:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 97:
				return 2
			case 98:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 3
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return 3
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 69:
				return -1
			case 76:
				return 4
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 108:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 69:
				return 5
			case 76:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 101:
				return 5
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Ee][Mm][Pp][Oo][Rr][Aa][Rr][Yy]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return 3
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return 3
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return 4
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return 4
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return 5
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return 5
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 6
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 6
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return 7
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return 8
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return 8
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return 9
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 77:
				return -1
			case 79:
				return -1
			case 80:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 109:
				return -1
			case 111:
				return -1
			case 112:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Ee][Rr][Mm][Ii][Nn][Aa][Tt][Ee][Dd]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 2
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 2
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 4
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 4
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return 5
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return 5
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 7
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 8
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 8
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 9
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 9
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 10
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return 10
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Ee][Xx][Tt]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 84:
				return 1
			case 88:
				return -1
			case 101:
				return -1
			case 116:
				return 1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return 2
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 84:
				return -1
			case 88:
				return 3
			case 101:
				return -1
			case 116:
				return -1
			case 120:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 84:
				return 4
			case 88:
				return -1
			case 101:
				return -1
			case 116:
				return 4
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Tt][Hh][Ee][Nn]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 84:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return 2
			case 78:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 3
			case 72:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 101:
				return 3
			case 104:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return 4
			case 84:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Tt][Ii][Mm][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 2
			case 77:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 3
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 3
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Tt][Ii][Mm][Ee][Ss][Tt][Aa][Mm][Pp]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return 2
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 3
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 3
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 4
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return 4
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return 5
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return 5
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 6
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return 7
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 8
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 8
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return 9
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return 9
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ii][Nn][Tt]1|[Tt][Ii][Nn][Yy][Ii][Nn][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return 1
			case 78:
				return -1
			case 84:
				return 2
			case 89:
				return -1
			case 105:
				return 1
			case 110:
				return -1
			case 116:
				return 2
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return 9
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return 9
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return 3
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return 3
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return 4
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return 4
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return 5
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return 6
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return 6
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return 7
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return 7
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 8
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 8
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 10
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 10
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return 11
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 49:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Ii][Nn][Yy][Tt][Ee][Xx][Tt]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 1
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 2
			case 78:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return 2
			case 110:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 3
			case 84:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 3
			case 116:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 89:
				return 4
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			case 121:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 5
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 5
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return 6
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 88:
				return 7
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 120:
				return 7
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return 8
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return 8
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 84:
				return -1
			case 88:
				return -1
			case 89:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 116:
				return -1
			case 120:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Oo]
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 84:
				return 1
			case 111:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return 2
			case 84:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 84:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Tt][Rr][Aa][Ii][Ll][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return 3
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return 4
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return 4
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return 6
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return 6
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return 7
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return 7
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 8
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return 8
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Rr][Ii][Gg][Gg][Ee][Rr]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 3
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 3
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 4
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return 4
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return 5
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return 5
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 71:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return 6
			case 103:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 82:
				return 7
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 114:
				return 7
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Dd][Oo]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return 1
			case 100:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 100:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 85:
				return -1
			case 100:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Ii][Oo][Nn]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return 1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 105:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return 5
			case 79:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return 5
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Ii][Qq][Uu][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 85:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 81:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 3
			case 78:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return 3
			case 110:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 81:
				return 4
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 113:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 85:
				return 5
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 117:
				return 5
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 6
			case 73:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 101:
				return 6
			case 105:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 81:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 113:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Ll][Oo][Cc][Kk]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return 1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return 4
			case 85:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return 4
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 5
			case 75:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return 5
			case 107:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return 6
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 107:
				return 6
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 75:
				return -1
			case 76:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 107:
				return -1
			case 108:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Ss][Ii][Gg][Nn][Ee][Dd]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return 1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 2
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 2
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 3
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return 4
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return 4
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return 5
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return 5
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return 7
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return 7
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return 8
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return 8
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 68:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Pp][Dd][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return 1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return 2
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return 2
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 3
			case 69:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return 3
			case 101:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 4
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return 4
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 6
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 6
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 80:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 112:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Ss][Aa][Gg][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 83:
				return -1
			case 85:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 115:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 83:
				return 2
			case 85:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 115:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 71:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 103:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 71:
				return 4
			case 83:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 103:
				return 4
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 5
			case 71:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 101:
				return 5
			case 103:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 71:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 103:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Ss][Ee]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return -1
			case 85:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return 2
			case 85:
				return -1
			case 101:
				return -1
			case 115:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 3
			case 83:
				return -1
			case 85:
				return -1
			case 101:
				return 3
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Uu][Ss][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return 1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return 2
			case 85:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 73:
				return 3
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 105:
				return 3
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 4
			case 83:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 4
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return 5
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 103:
				return 5
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Tt][Cc]_[Dd][Aa][Tt][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return 1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return 2
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return 5
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return 5
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 6
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return 7
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return 7
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return 8
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return 8
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Tt][Cc]_[Tt][Ii][Mm][Ee]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return 1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 2
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 4
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 7
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 7
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Uu][Tt][Cc]_[Tt][Ii][Mm][Ee][Ss][Tt][Aa][Mm][Pp]
	{[]bool{false, false, false, false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 2
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 2
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 3
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return 3
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 4
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return 6
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return 6
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 7
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 7
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 8
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 8
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return 9
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return 9
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return 10
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return 10
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 11
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 11
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return 12
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return 12
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return 13
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return 13
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 73:
				return -1
			case 77:
				return -1
			case 80:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 112:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Vv][Aa][Ll][Uu][Ee][Ss]?
	{[]bool{false, false, false, false, false, true, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 86:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 118:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 86:
				return -1
			case 97:
				return 2
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return 3
			case 83:
				return -1
			case 85:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return 3
			case 115:
				return -1
			case 117:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 85:
				return 4
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 117:
				return 4
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 5
			case 76:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return 5
			case 108:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return 6
			case 85:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return 6
			case 117:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 85:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			case 117:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Vv][Aa][Rr][Bb][Ii][Nn][Aa][Rr][Yy]
	{[]bool{false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return 1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return 1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return 2
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 4
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return 4
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 7
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return 7
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 8
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 8
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return 9
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Vv][Aa][Rr][Cc][Hh][Aa][Rr]([Aa][Cc][Tt][Ee][Rr])?
	{[]bool{false, false, false, false, false, false, false, true, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return 1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return 2
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 3
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 3
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 4
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return 4
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return 5
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return 5
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return 6
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 7
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 7
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 8
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return 8
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return 9
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return 9
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return 10
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return 10
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return 11
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return 11
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 12
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 12
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 67:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 86:
				return -1
			case 97:
				return -1
			case 99:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 118:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Vv][Aa][Rr][Yy][Ii][Nn][Gg]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return 1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return 1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return 2
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 3
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 3
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return 4
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return 5
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return 5
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 6
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 6
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return 7
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return 7
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 86:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 118:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ww][Hh][Ee][Nn]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 87:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 119:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return 2
			case 78:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 110:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 3
			case 72:
				return -1
			case 78:
				return -1
			case 87:
				return -1
			case 101:
				return 3
			case 104:
				return -1
			case 110:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return 4
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return 4
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 78:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 110:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ww][Hh][Ee][Rr][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 87:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 119:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return 2
			case 82:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 114:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 3
			case 72:
				return -1
			case 82:
				return -1
			case 87:
				return -1
			case 101:
				return 3
			case 104:
				return -1
			case 114:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return 4
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return 4
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 5
			case 72:
				return -1
			case 82:
				return -1
			case 87:
				return -1
			case 101:
				return 5
			case 104:
				return -1
			case 114:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 82:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 114:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ww][Hh][Ii][Ll][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 87:
				return 1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 119:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return 2
			case 73:
				return -1
			case 76:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return 2
			case 105:
				return -1
			case 108:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return 3
			case 76:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return 3
			case 108:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 76:
				return 4
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 108:
				return 4
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 5
			case 72:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 87:
				return -1
			case 101:
				return 5
			case 104:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 72:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Ww][Ii][Tt][Hh]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 73:
				return -1
			case 84:
				return -1
			case 87:
				return 1
			case 104:
				return -1
			case 105:
				return -1
			case 116:
				return -1
			case 119:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 73:
				return 2
			case 84:
				return -1
			case 87:
				return -1
			case 104:
				return -1
			case 105:
				return 2
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 73:
				return -1
			case 84:
				return 3
			case 87:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 116:
				return 3
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return 4
			case 73:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 104:
				return 4
			case 105:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 72:
				return -1
			case 73:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 104:
				return -1
			case 105:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Ww][Rr][Ii][Tt][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return 1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return 3
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 105:
				return 3
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return 4
			case 87:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return 4
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 5
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 101:
				return 5
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 73:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 87:
				return -1
			case 101:
				return -1
			case 105:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [Xx][Oo][Rr]
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 82:
				return -1
			case 88:
				return 1
			case 111:
				return -1
			case 114:
				return -1
			case 120:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return 2
			case 82:
				return -1
			case 88:
				return -1
			case 111:
				return 2
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 82:
				return 3
			case 88:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			case 120:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 79:
				return -1
			case 82:
				return -1
			case 88:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 120:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// [Yy][Ee][Aa][Rr]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 89:
				return 1
			case 97:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 121:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 82:
				return 4
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 114:
				return 4
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 82:
				return -1
			case 89:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Yy][Ee][Aa][Rr]_[Mm][Oo][Nn][Tt][Hh]
	{[]bool{false, false, false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return 1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 2
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return 2
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 3
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return 3
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return 4
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return 4
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return 5
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return 6
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return 6
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return 7
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return 7
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return 8
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return 8
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return 9
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return 9
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return 10
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return 10
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 72:
				return -1
			case 77:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 89:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 104:
				return -1
			case 109:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 121:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Zz][Ee][Rr][Oo][Ff][Ii][Ll][Ll]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return 1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 2
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return 2
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return 3
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return 3
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return 4
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return 4
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return 5
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return 5
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return 6
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return 6
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return 7
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return 7
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return 8
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return 8
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 70:
				return -1
			case 73:
				return -1
			case 76:
				return -1
			case 79:
				return -1
			case 82:
				return -1
			case 90:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 105:
				return -1
			case 108:
				return -1
			case 111:
				return -1
			case 114:
				return -1
			case 122:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Rr][Uu][Ee]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 85:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return 3
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return 4
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return 4
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 69:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 101:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Uu][Nn][Kk][Nn][Oo][Ww][Nn]
	{[]bool{false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return 1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return 1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return 2
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return 2
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return 3
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return 3
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return 4
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return 4
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return -1
			case 79:
				return 5
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 111:
				return 5
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return 6
			case 107:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return 7
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return 7
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 75:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 85:
				return -1
			case 87:
				return -1
			case 107:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 117:
				return -1
			case 119:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Ff][Aa][Ll][Ss][Ee]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 70:
				return 1
			case 76:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 102:
				return 1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 97:
				return 2
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return 3
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return 3
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 83:
				return 4
			case 97:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 115:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return 5
			case 70:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return 5
			case 102:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 69:
				return -1
			case 70:
				return -1
			case 76:
				return -1
			case 83:
				return -1
			case 97:
				return -1
			case 101:
				return -1
			case 102:
				return -1
			case 108:
				return -1
			case 115:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [-+&~|^\/%\*(),.;!]
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 33:
				return 1
			case 37:
				return 1
			case 38:
				return 1
			case 40:
				return 1
			case 41:
				return 1
			case 42:
				return 1
			case 43:
				return 1
			case 44:
				return 1
			case 45:
				return 1
			case 46:
				return 1
			case 47:
				return 1
			case 59:
				return 1
			case 94:
				return 1
			case 124:
				return 1
			case 126:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 33:
				return -1
			case 37:
				return -1
			case 38:
				return -1
			case 40:
				return -1
			case 41:
				return -1
			case 42:
				return -1
			case 43:
				return -1
			case 44:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 47:
				return -1
			case 59:
				return -1
			case 94:
				return -1
			case 124:
				return -1
			case 126:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},

	// &&
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 38:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 38:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 38:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// \|\|
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 124:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 124:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 124:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// =
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 61:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 61:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},

	// <=>
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 60:
				return 1
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			case 61:
				return 2
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			case 61:
				return -1
			case 62:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// >=
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 61:
				return -1
			case 62:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 61:
				return 2
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// >
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 62:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 62:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},

	// <=
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 60:
				return 1
			case 61:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			case 61:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			case 61:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// <
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 60:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},

	// !=|<>
	{[]bool{false, false, false, true, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 33:
				return 1
			case 60:
				return 2
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 33:
				return -1
			case 60:
				return -1
			case 61:
				return 4
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 33:
				return -1
			case 60:
				return -1
			case 61:
				return -1
			case 62:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 33:
				return -1
			case 60:
				return -1
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 33:
				return -1
			case 60:
				return -1
			case 61:
				return -1
			case 62:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// <<
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 60:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 60:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// >>
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 62:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 62:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 62:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// [Xx]'[0-9[A-Fa-f]+'|0[Xx][0-9A-Fa-f]+
	{[]bool{false, false, false, false, false, true, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 1
			case 88:
				return 2
			case 91:
				return -1
			case 120:
				return 2
			}
			switch {
			case 48 <= r && r <= 57:
				return -1
			case 65 <= r && r <= 70:
				return -1
			case 97 <= r && r <= 102:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return -1
			case 88:
				return 6
			case 91:
				return -1
			case 120:
				return 6
			}
			switch {
			case 48 <= r && r <= 57:
				return -1
			case 65 <= r && r <= 70:
				return -1
			case 97 <= r && r <= 102:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return 3
			case 48:
				return -1
			case 88:
				return -1
			case 91:
				return -1
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return -1
			case 65 <= r && r <= 70:
				return -1
			case 97 <= r && r <= 102:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 4
			case 88:
				return -1
			case 91:
				return 4
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 4
			case 65 <= r && r <= 70:
				return 4
			case 97 <= r && r <= 102:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return 5
			case 48:
				return 4
			case 88:
				return -1
			case 91:
				return 4
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 4
			case 65 <= r && r <= 70:
				return 4
			case 97 <= r && r <= 102:
				return 4
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return -1
			case 88:
				return -1
			case 91:
				return -1
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return -1
			case 65 <= r && r <= 70:
				return -1
			case 97 <= r && r <= 102:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 7
			case 88:
				return -1
			case 91:
				return -1
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 7
			case 65 <= r && r <= 70:
				return 7
			case 97 <= r && r <= 102:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 7
			case 88:
				return -1
			case 91:
				return -1
			case 120:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 7
			case 65 <= r && r <= 70:
				return 7
			case 97 <= r && r <= 102:
				return 7
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// 0[Bb][01]+|[Bb]'[01]+'
	{[]bool{false, false, false, false, false, true, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 1
			case 49:
				return -1
			case 66:
				return 2
			case 98:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return -1
			case 49:
				return -1
			case 66:
				return 6
			case 98:
				return 6
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return 3
			case 48:
				return -1
			case 49:
				return -1
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 4
			case 49:
				return 4
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return 5
			case 48:
				return 4
			case 49:
				return 4
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return -1
			case 49:
				return -1
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 7
			case 49:
				return 7
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 39:
				return -1
			case 48:
				return 7
			case 49:
				return 7
			case 66:
				return -1
			case 98:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// -?[0-9]+
	{[]bool{false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 45:
				return 1
			}
			switch {
			case 48 <= r && r <= 57:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 45:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 45:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 2
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// -?[0-9]+\.[0-9]*|-?\.[0-9]+|-?[0-9]+[Ee][-+]?[0-9]+|-?[0-9]+\.[0-9]*[Ee][-+]?[0-9]+|-?\.[0-9]*[Ee][-+]?[0-9]+
	{[]bool{false, false, false, false, true, false, false, true, false, true, false, true, false, true, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return 1
			case 46:
				return 2
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return 2
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return 12
			case 101:
				return 12
			}
			switch {
			case 48 <= r && r <= 57:
				return 13
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return 4
			case 69:
				return 5
			case 101:
				return 5
			}
			switch {
			case 48 <= r && r <= 57:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return 8
			case 101:
				return 8
			}
			switch {
			case 48 <= r && r <= 57:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return 6
			case 45:
				return 6
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return 10
			case 45:
				return 10
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return 8
			case 101:
				return 8
			}
			switch {
			case 48 <= r && r <= 57:
				return 9
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 11
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return 14
			case 45:
				return 14
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 15
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return 12
			case 101:
				return 12
			}
			switch {
			case 48 <= r && r <= 57:
				return 13
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 15
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 43:
				return -1
			case 45:
				return -1
			case 46:
				return -1
			case 69:
				return -1
			case 101:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return 15
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// '(\\.|''|[^'\n])*'
	{[]bool{false, false, true, false, false, false, true, false, false, true, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 10
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 39:
				return 6
			case 92:
				return 7
			}
			return 8
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 9
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 39:
				return 6
			case 92:
				return 7
			}
			return 8
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 9
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 2
			case 92:
				return 3
			}
			return 4
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// \"(\\.|\"\"|[^"\n])*\"
	{[]bool{false, false, true, false, false, false, true, false, false, true, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 10
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 34:
				return 6
			case 92:
				return 7
			}
			return 8
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 9
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 34:
				return 6
			case 92:
				return 7
			}
			return 8
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 2
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 9
			case 92:
				return 3
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 2
			case 92:
				return 3
			}
			return 4
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// '(\\.|[^'\n])*$
	{[]bool{false, false, false, false, true, false, false, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return 1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 39:
				return 5
			case 92:
				return 6
			}
			return 7
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return -1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 39:
				return 5
			case 92:
				return 6
			}
			return 7
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 39:
				return -1
			case 92:
				return 2
			}
			return 3
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, 4, 4, 4, -1, 4, 4, 4}, nil},

	// \"(\\.|[^"\n])*$
	{[]bool{false, false, false, false, true, false, false, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return 1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 34:
				return 5
			case 92:
				return 6
			}
			return 7
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return -1
			case 92:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return -1
			case 92:
				return 2
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return 5
			case 34:
				return 5
			case 92:
				return 6
			}
			return 7
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 34:
				return -1
			case 92:
				return 2
			}
			return 3
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, 4, 4, 4, -1, 4, 4, 4}, nil},

	// [Ss][Uu][Bb][Ss][Tt][Rr]([Ii][Nn][Gg])?
	{[]bool{false, false, false, false, false, false, true, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 2
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return 3
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return 3
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return 4
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return 4
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return 6
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return 6
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return 7
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return 7
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return 8
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return 8
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return 9
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return 9
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 66:
				return -1
			case 71:
				return -1
			case 73:
				return -1
			case 78:
				return -1
			case 82:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 98:
				return -1
			case 103:
				return -1
			case 105:
				return -1
			case 110:
				return -1
			case 114:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Tt][Rr][Ii][Mm]
	{[]bool{false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 77:
				return -1
			case 82:
				return -1
			case 84:
				return 1
			case 105:
				return -1
			case 109:
				return -1
			case 114:
				return -1
			case 116:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 77:
				return -1
			case 82:
				return 2
			case 84:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 114:
				return 2
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return 3
			case 77:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 105:
				return 3
			case 109:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 77:
				return 4
			case 82:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 109:
				return 4
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 73:
				return -1
			case 77:
				return -1
			case 82:
				return -1
			case 84:
				return -1
			case 105:
				return -1
			case 109:
				return -1
			case 114:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Ee]_[Aa][Dd][Dd]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return 3
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return 4
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return 5
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 6
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return 6
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 7
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return 7
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return 8
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return 8
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 84:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 116:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Dd][Aa][Tt][Ee]_[Ss][Uu][Bb]
	{[]bool{false, false, false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return 1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return 1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return 2
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return 2
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return 3
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return 3
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return 4
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return 4
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return 5
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return 6
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return 6
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return 7
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return 7
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return 8
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return 8
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 65:
				return -1
			case 66:
				return -1
			case 68:
				return -1
			case 69:
				return -1
			case 83:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 95:
				return -1
			case 97:
				return -1
			case 98:
				return -1
			case 100:
				return -1
			case 101:
				return -1
			case 115:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1, -1, -1}, nil},

	// [Cc][Oo][Uu][Nn][Tt]
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 67:
				return 1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return 1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 78:
				return -1
			case 79:
				return 2
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 110:
				return -1
			case 111:
				return 2
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return 3
			case 99:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return 3
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 78:
				return 4
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 110:
				return 4
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return 5
			case 85:
				return -1
			case 99:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return 5
			case 117:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 67:
				return -1
			case 78:
				return -1
			case 79:
				return -1
			case 84:
				return -1
			case 85:
				return -1
			case 99:
				return -1
			case 110:
				return -1
			case 111:
				return -1
			case 116:
				return -1
			case 117:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// [A-Za-z][A-Za-z0-9_]*
	{[]bool{false, true, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 95:
				return -1
			}
			switch {
			case 48 <= r && r <= 57:
				return -1
			case 65 <= r && r <= 90:
				return 1
			case 97 <= r && r <= 122:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 95:
				return 2
			}
			switch {
			case 48 <= r && r <= 57:
				return 2
			case 65 <= r && r <= 90:
				return 2
			case 97 <= r && r <= 122:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 95:
				return 2
			}
			switch {
			case 48 <= r && r <= 57:
				return 2
			case 65 <= r && r <= 90:
				return 2
			case 97 <= r && r <= 122:
				return 2
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1}, nil},

	// `[^`\/\\.\n]+`
	{[]bool{false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 46:
				return -1
			case 47:
				return -1
			case 92:
				return -1
			case 96:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 46:
				return -1
			case 47:
				return -1
			case 92:
				return -1
			case 96:
				return -1
			}
			return 2
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 46:
				return -1
			case 47:
				return -1
			case 92:
				return -1
			case 96:
				return 3
			}
			return 2
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 46:
				return -1
			case 47:
				return -1
			case 92:
				return -1
			case 96:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// #[^\n]*\n
	{[]bool{false, false, true, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 35:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 2
			case 35:
				return 3
			}
			return 3
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 35:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 2
			case 35:
				return 3
			}
			return 3
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1}, nil},

	// --[ \t][^\n]*\n
	{[]bool{false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 45:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 45:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 3
			case 10:
				return -1
			case 32:
				return 3
			case 45:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return 4
			case 10:
				return 5
			case 32:
				return 4
			case 45:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 9:
				return 4
			case 10:
				return 5
			case 32:
				return 4
			case 45:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			case 45:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1}, nil},

	// \/\/[^\n]*\n
	{[]bool{false, false, false, true, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 47:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 47:
				return 2
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 10:
				return -1
			case 47:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 10:
				return 3
			case 47:
				return 4
			}
			return 4
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1}, nil},

	// \/\*([^*]|\*[^\/])*\*\/
	{[]bool{false, false, false, false, false, false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 42:
				return -1
			case 47:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 42:
				return 2
			case 47:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 42:
				return 5
			case 47:
				return 6
			}
			return 5
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 42:
				return -1
			case 47:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, nil},

	// [ \t\n]
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 9:
				return 1
			case 10:
				return 1
			case 32:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 9:
				return -1
			case 10:
				return -1
			case 32:
				return -1
			}
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},

	// \/\*([^*]|\*[^\/])*$
	{[]bool{false, false, false, false, false, true, false}, []func(rune) int{ // Transitions
		func(r rune) int {
			switch r {
			case 42:
				return -1
			case 47:
				return 1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 42:
				return 2
			case 47:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 42:
				return 6
			case 47:
				return -1
			}
			return 6
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
		func(r rune) int {
			switch r {
			case 42:
				return -1
			case 47:
				return -1
			}
			return -1
		},
		func(r rune) int {
			switch r {
			case 42:
				return 3
			case 47:
				return 4
			}
			return 4
		},
	}, []int{ /* Start-of-input transitions */ -1, -1, -1, -1, -1, -1, -1}, []int{ /* End-of-input transitions */ -1, -1, 5, -1, 5, -1, 5}, nil},

	// .
	{[]bool{false, true}, []func(rune) int{ // Transitions
		func(r rune) int {
			return 1
		},
		func(r rune) int {
			return -1
		},
	}, []int{ /* Start-of-input transitions */ -1, -1}, []int{ /* End-of-input transitions */ -1, -1}, nil},
}

func NewLexer(in io.Reader) *Lexer {
	return NewLexerWithInit(in, nil)
}

func (yyLex *Lexer) Stop() {
	yyLex.ch_stop <- true
}

// Text returns the matched text.
func (yylex *Lexer) Text() string {
	return yylex.stack[len(yylex.stack)-1].s
}

// Line returns the current line number.
// The first line is 0.
func (yylex *Lexer) Line() int {
	if len(yylex.stack) == 0 {
		return 0
	}
	return yylex.stack[len(yylex.stack)-1].line
}

// Column returns the current column number.
// The first column is 0.
func (yylex *Lexer) Column() int {
	if len(yylex.stack) == 0 {
		return 0
	}
	return yylex.stack[len(yylex.stack)-1].column
}

func (yylex *Lexer) next(lvl int) int {
	if lvl == len(yylex.stack) {
		l, c := 0, 0
		if lvl > 0 {
			l, c = yylex.stack[lvl-1].line, yylex.stack[lvl-1].column
		}
		yylex.stack = append(yylex.stack, frame{0, "", l, c})
	}
	if lvl == len(yylex.stack)-1 {
		p := &yylex.stack[lvl]
		*p = <-yylex.ch
		yylex.stale = false
	} else {
		yylex.stale = true
	}
	return yylex.stack[lvl].i
}
func (yylex *Lexer) pop() {
	yylex.stack = yylex.stack[:len(yylex.stack)-1]
}
func (yylex Lexer) Error(e string) {
	panic(e)
}

// Lex runs the lexer. Always returns 0.
// When the -s option is given, this function is not generated;
// instead, the NN_FUN macro runs the lexer.
func (yylex *Lexer) Lex(lval *yySymType) int {
OUTER0:
	for {
		switch yylex.next(0) {
		case 0:
			{
				return ADD
			}
		case 1:
			{
				return ALL
			}
		case 2:
			{
				return ALTER
			}
		case 3:
			{
				return ANALYZE
			}
		case 4:
			{
				return ANDOP
			}
		case 5:
			{
				return AND
			}
		case 6:
			{
				return ANY
			}
		case 7:
			{
				return AS
			}
		case 8:
			{
				return ASC
			}
		case 9:
			{
				return AUTO_INCREMENT
			}
		case 10:
			{
				return BEFORE
			}
		case 11:
			{
				return BETWEEN
			}
		case 12:
			{
				return BIGINT
			}
		case 13:
			{
				return BINARY
			}
		case 14:
			{
				return BIT
			}
		case 15:
			{
				return BLOB
			}
		case 16:
			{
				return BOTH
			}
		case 17:
			{
				return BY
			}
		case 18:
			{
				return CALL
			}
		case 19:
			{
				return CASCADE
			}
		case 20:
			{
				return CASE
			}
		case 21:
			{
				return CHANGE
			}
		case 22:
			{
				return CHAR
			}
		case 23:
			{
				return CHECK
			}
		case 24:
			{
				return COLLATE
			}
		case 25:
			{
				return COLUMN
			}
		case 26:
			{
				return COMMENT
			}
		case 27:
			{
				return CONDITION
			}
		case 28:
			{
				return CONSTRAINT
			}
		case 29:
			{
				return CONTINUE
			}
		case 30:
			{
				return CONVERT
			}
		case 31:
			{
				return CREATE
			}
		case 32:
			{
				return CROSS
			}
		case 33:
			{
				return CURRENT_DATE
			}
		case 34:
			{
				return CURRENT_TIME
			}
		case 35:
			{
				return CURRENT_TIMESTAMP
			}
		case 36:
			{
				return CURRENT_USER
			}
		case 37:
			{
				return CURSOR
			}
		case 38:
			{
				return DATABASE
			}
		case 39:
			{
				return DATABASES
			}
		case 40:
			{
				return DATE
			}
		case 41:
			{
				return DATETIME
			}
		case 42:
			{
				return DAY_HOUR
			}
		case 43:
			{
				return DAY_MICROSECOND
			}
		case 44:
			{
				return DAY_MINUTE
			}
		case 45:
			{
				return DAY_SECOND
			}
		case 46:
			{
				return DECIMAL
			}
		case 47:
			{
				return DECLARE
			}
		case 48:
			{
				return DEFAULT
			}
		case 49:
			{
				return DELAYED
			}
		case 50:
			{
				return DELETE
			}
		case 51:
			{
				return DESC
			}
		case 52:
			{
				return DESCRIBE
			}
		case 53:
			{
				return DETERMINISTIC
			}
		case 54:
			{
				return DISTINCT
			}
		case 55:
			{
				return DISTINCTROW
			}
		case 56:
			{
				return DIV
			}
		case 57:
			{
				return DOUBLE
			}
		case 58:
			{
				return DROP
			}
		case 59:
			{
				return DUAL
			}
		case 60:
			{
				return EACH
			}
		case 61:
			{
				return ELSE
			}
		case 62:
			{
				return ELSEIF
			}
		case 63:
			{
				return END
			}
		case 64:
			{
				return ENUM
			}
		case 65:
			{
				return ESCAPED
			}
		case 66:
			{
				lval.subtok = 0
				return EXISTS
			}
		case 67:
			{
				lval.subtok = 1
				return EXISTS
			}
		case 68:
			{
				return EXIT
			}
		case 69:
			{
				return EXPLAIN
			}
		case 70:
			{
				return FETCH
			}
		case 71:
			{
				return FLOAT
			}
		case 72:
			{
				return FOR
			}
		case 73:
			{
				return FORCE
			}
		case 74:
			{
				return FOREIGN
			}
		case 75:
			{
				return FROM
			}
		case 76:
			{
				return FULLTEXT
			}
		case 77:
			{
				return GRANT
			}
		case 78:
			{
				return GROUP
			}
		case 79:
			{
				return HAVING
			}
		case 80:
			{
				return HIGH_PRIORITY
			}
		case 81:
			{
				return HOUR_MICROSECOND
			}
		case 82:
			{
				return HOUR_MINUTE
			}
		case 83:
			{
				return HOUR_SECOND
			}
		case 84:
			{
				return IF
			}
		case 85:
			{
				return IGNORE
			}
		case 86:
			{
				return IN
			}
		case 87:
			{
				return INFILE
			}
		case 88:
			{
				return INNER
			}
		case 89:
			{
				return INOUT
			}
		case 90:
			{
				return INSENSITIVE
			}
		case 91:
			{
				return INSERT
			}
		case 92:
			{
				return INTEGER
			}
		case 93:
			{
				return INTERVAL
			}
		case 94:
			{
				return INTO
			}
		case 95:
			{
				return IS
			}
		case 96:
			{
				return ITERATE
			}
		case 97:
			{
				return JOIN
			}
		case 98:
			{
				return KEY
			}
		case 99:
			{
				return KEYS
			}
		case 100:
			{
				return KILL
			}
		case 101:
			{
				return LEADING
			}
		case 102:
			{
				return LEAVE
			}
		case 103:
			{
				return LEFT
			}
		case 104:
			{
				return LIKE
			}
		case 105:
			{
				return LIMIT
			}
		case 106:
			{
				return LINES
			}
		case 107:
			{
				return LOAD
			}
		case 108:
			{
				return LOCALTIME
			}
		case 109:
			{
				return LOCALTIMESTAMP
			}
		case 110:
			{
				return LOCK
			}
		case 111:
			{
				return LONG
			}
		case 112:
			{
				return LONGBLOB
			}
		case 113:
			{
				return LONGTEXT
			}
		case 114:
			{
				return LOOP
			}
		case 115:
			{
				return LOW_PRIORITY
			}
		case 116:
			{
				return MATCH
			}
		case 117:
			{
				return MEDIUMBLOB
			}
		case 118:
			{
				return MEDIUMINT
			}
		case 119:
			{
				return MEDIUMTEXT
			}
		case 120:
			{
				return MINUTE_MICROSECOND
			}
		case 121:
			{
				return MINUTE_SECOND
			}
		case 122:
			{
				return MOD
			}
		case 123:
			{
				return MODIFIES
			}
		case 124:
			{
				return NATURAL
			}
		case 125:
			{
				return NOT
			}
		case 126:
			{
				return NO_WRITE_TO_BINLOG
			}
		case 127:
			{
				return NULLX
			}
		case 128:
			{
				return NUMBER
			}
		case 129:
			{
				return ON
			}
		case 130:
			{
				return ONDUPLICATE
			}
		case 131:
			{
				return OPTIMIZE
			}
		case 132:
			{
				return OPTION
			}
		case 133:
			{
				return OPTIONALLY
			}
		case 134:
			{
				return OR
			}
		case 135:
			{
				return ORDER
			}
		case 136:
			{
				return OUT
			}
		case 137:
			{
				return OUTER
			}
		case 138:
			{
				return OUTFILE
			}
		case 139:
			{
				return PRECISION
			}
		case 140:
			{
				return PRIMARY
			}
		case 141:
			{
				return PROCEDURE
			}
		case 142:
			{
				return PURGE
			}
		case 143:
			{
				return QUICK
			}
		case 144:
			{
				return READ
			}
		case 145:
			{
				return READS
			}
		case 146:
			{
				return REAL
			}
		case 147:
			{
				return REFERENCES
			}
		case 148:
			{
				return REGEXP
			}
		case 149:
			{
				return RELEASE
			}
		case 150:
			{
				return RENAME
			}
		case 151:
			{
				return REPEAT
			}
		case 152:
			{
				return REPLACE
			}
		case 153:
			{
				return REQUIRE
			}
		case 154:
			{
				return RESTRICT
			}
		case 155:
			{
				return RETURN
			}
		case 156:
			{
				return REVOKE
			}
		case 157:
			{
				return RIGHT
			}
		case 158:
			{
				return ROLLUP
			}
		case 159:
			{
				return SCHEMA
			}
		case 160:
			{
				return SCHEMAS
			}
		case 161:
			{
				return SECOND_MICROSECOND
			}
		case 162:
			{
				return SELECT
			}
		case 163:
			{
				return SENSITIVE
			}
		case 164:
			{
				return SEPARATOR
			}
		case 165:
			{
				return SET
			}
		case 166:
			{
				return SHOW
			}
		case 167:
			{
				return SMALLINT
			}
		case 168:
			{
				return SOME
			}
		case 169:
			{
				return SONAME
			}
		case 170:
			{
				return SPATIAL
			}
		case 171:
			{
				return SPECIFIC
			}
		case 172:
			{
				return SQL
			}
		case 173:
			{
				return SQLEXCEPTION
			}
		case 174:
			{
				return SQLSTATE
			}
		case 175:
			{
				return SQLWARNING
			}
		case 176:
			{
				return SQL_BIG_RESULT
			}
		case 177:
			{
				return SQL_CALC_FOUND_ROWS
			}
		case 178:
			{
				return SQL_SMALL_RESULT
			}
		case 179:
			{
				return SSL
			}
		case 180:
			{
				return STARTING
			}
		case 181:
			{
				return STRAIGHT_JOIN
			}
		case 182:
			{
				return TABLE
			}
		case 183:
			{
				return TEMPORARY
			}
		case 184:
			{
				return TERMINATED
			}
		case 185:
			{
				return TEXT
			}
		case 186:
			{
				return THEN
			}
		case 187:
			{
				return TIME
			}
		case 188:
			{
				return TIMESTAMP
			}
		case 189:
			{
				return TINYINT
			}
		case 190:
			{
				return TINYTEXT
			}
		case 191:
			{
				return TO
			}
		case 192:
			{
				return TRAILING
			}
		case 193:
			{
				return TRIGGER
			}
		case 194:
			{
				return UNDO
			}
		case 195:
			{
				return UNION
			}
		case 196:
			{
				return UNIQUE
			}
		case 197:
			{
				return UNLOCK
			}
		case 198:
			{
				return UNSIGNED
			}
		case 199:
			{
				return UPDATE
			}
		case 200:
			{
				return USAGE
			}
		case 201:
			{
				return USE
			}
		case 202:
			{
				return USING
			}
		case 203:
			{
				return UTC_DATE
			}
		case 204:
			{
				return UTC_TIME
			}
		case 205:
			{
				return UTC_TIMESTAMP
			}
		case 206:
			{
				return VALUES
			}
		case 207:
			{
				return VARBINARY
			}
		case 208:
			{
				return VARCHAR
			}
		case 209:
			{
				return VARYING
			}
		case 210:
			{
				return WHEN
			}
		case 211:
			{
				return WHERE
			}
		case 212:
			{
				return WHILE
			}
		case 213:
			{
				return WITH
			}
		case 214:
			{
				return WRITE
			}
		case 215:
			{
				return XOR
			}
		case 216:
			{
				return YEAR
			}
		case 217:
			{
				return YEAR_MONTH
			}
		case 218:
			{
				return ZEROFILL
			}
		case 219:
			{
				lval.intval = 1
				return BOOL
			}
		case 220:
			{
				lval.intval = -1
				return BOOL
			}
		case 221:
			{
				lval.intval = 0
				return BOOL
			}
		case 222:
			{
				return int(yylex.Text()[0])
			}
		case 223:
			{
				return ANDOP
			}
		case 224:
			{
				return OR
			}
		case 225:
			{
				lval.subtok = 4
				return COMPARISON
			}
		case 226:
			{
				lval.subtok = 12
				return COMPARISON
			}
		case 227:
			{
				lval.subtok = 6
				return COMPARISON
			}
		case 228:
			{
				lval.subtok = 2
				return COMPARISON
			}
		case 229:
			{
				lval.subtok = 5
				return COMPARISON
			}
		case 230:
			{
				lval.subtok = 1
				return COMPARISON
			}
		case 231:
			{
				lval.subtok = 3
				return COMPARISON
			}
		case 232:
			{
				lval.subtok = 1
				return SHIFT
			}
		case 233:
			{
				lval.subtok = 2
				return SHIFT
			}
		case 234:
			{
				lval.strval = yylex.Text()
				return STRING
			}
		case 235:
			{
				lval.strval = yylex.Text()
				return STRING
			}
		case 236:
			{
				i, _ := strconv.Atoi(yylex.Text())
				lval.intval = i
				return INTNUM
			}
		case 237:
			{
				i, _ := strconv.ParseFloat(yylex.Text(), 64)
				lval.floatval = i
				return APPROXNUM
			}
		case 238:
			{
				lval.strval = yylex.Text()[1 : len(yylex.Text())-2]
				return STRING
			}
		case 239:
			{
				lval.strval = yylex.Text()[1 : len(yylex.Text())-2]
				return STRING
			}
		case 240:
			{
				fmt.Println("Lexer: Unterminated string:", yylex.Text())
			}
		case 241:
			{
				fmt.Println("Lexer: Unterminated string:", yylex.Text())
			}
		case 242:
			{
				return FSUBSTRING
			}
		case 243:
			{
				return FTRIM
			}
		case 244:
			{
				return FDATE_ADD
			}
		case 245:
			{
				return FDATE_SUB
			}
		case 246:
			{
				return FCOUNT
			}
		case 247:
			{
				lval.strval = yylex.Text()
				return NAME
			}
		case 248:
			{
				lval.strval = yylex.Text()[1 : len(yylex.Text())-2]
				return NAME
			}
		case 249:
			{
			}
		case 250:
			{
			}
		case 251:
			{
			}
		case 252:
			{
			}
		case 253:
			{
			}
		case 254:
			{
				fmt.Println("Lexer: Unterminated string:", yylex.Text())
			}
		case 255:
			{
				fmt.Println("Lexer: invalid charactor:", yylex.Text())
			}
		default:
			break OUTER0
		}
		continue
	}
	yylex.pop()

	return 0
}
func main() {
	defer func() {
		if e := recover(); e != nil {
			err := errors.New(fmt.Sprint(e))
			fmt.Println(err)
		}
	}()
	lex := NewLexer(os.Stdin)
	yyParse(lex)
}
