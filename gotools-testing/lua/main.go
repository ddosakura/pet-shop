package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

var (
	logMode = false
)

func main() {
	var lex *Lexer
	var filename string
	defer func() {
		if e := recover(); e != nil {
			err := errors.New(fmt.Sprint(e))
			fmt.Printf("%s:%d:%d: %s\n", filename, lex.Line()+1, lex.Column()+1, err.Error())
		}
	}()
	if len(os.Args) == 1 {
		filename = "stdin"
		logMode = true

		r, w := io.Pipe()
		lex = NewLexer(r)
		w.Write([]byte("\nprint(_VERSION)\n"))
		go io.Copy(w, os.Stdin)
	} else {
		f, e := os.Open(os.Args[1])
		if e != nil {
			fmt.Printf("%s\n", e.Error())
		} else {
			lex = NewLexer(f)
		}
	}
	if lex != nil {
		yyParse(lex)
	}
}

var (
	vals  map[string]interface{}
	funcs map[string]func(...interface{}) interface{}
)

func init() {
	vals = map[string]interface{}{
		"_VERSION": "Lua 5.3 (BETA) ddosakura",
	}
	funcs = map[string]func(...interface{}) interface{}{
		"print": func(args ...interface{}) interface{} {
			for i, a := range args {
				if i == 0 {
					fmt.Printf("%v", a)
				} else {
					fmt.Printf("\t%v", a)
				}
			}
			fmt.Println()
			return nil
		},
	}
}

func unquote(s string) string {
	a, e := strconv.Unquote(`"` + s + `"`)
	if e != nil {
		panic(e)
	}
	return a
}
