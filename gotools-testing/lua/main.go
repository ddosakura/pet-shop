package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

func main() {
	var lex *Lexer
	defer func() {
		if e := recover(); e != nil {
			err := errors.New(fmt.Sprint(e))
			fmt.Printf("./main.lua:%d:%d: %s\n", lex.Line()+1, lex.Column()+1, err.Error())
		}
	}()
	lex = NewLexer(os.Stdin)
	yyParse(lex)
}

var (
	vals  map[string]interface{}
	funcs map[string]func(...interface{}) interface{}
)

func init() {
	vals = map[string]interface{}{
		"_VERSION": "Lua 5.3 (BETA)",
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

	// println("Lua 5.3 (BETA) ddosakura")
}

func unquote(s string) string {
	a, e := strconv.Unquote(`"` + s + `"`)
	if e != nil {
		panic(e)
	}
	return a
}
