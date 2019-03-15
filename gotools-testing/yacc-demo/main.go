package main

import (
	"errors"
	"fmt"
	"os"
)

var (
	nLines = 1
	nChars int
)

func main() {
	defer func() {
		if e := recover(); e != nil {
			err := errors.New(fmt.Sprint(e))
			fmt.Printf("./demo:%d:%d: %s\n", nLines, nChars, err.Error())
		}
	}()
	lex := NewLexer(os.Stdin)
	yyParse(lex)
}
