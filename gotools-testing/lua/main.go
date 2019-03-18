package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
)

var (
	logMode = false
)

func main() {
	var lex *Lexer
	var filename string
	if len(os.Args) == 1 {
		filename = "stdin"
		logMode = true

		r, w := io.Pipe()
		lex = NewLexer(r)
		w.Write([]byte("\nprint(_VERSION)\n"))
		go io.Copy(w, os.Stdin)
	} else {
		filename = os.Args[1]
		if !path.IsAbs(filename) {
			filename = "./" + filename
		}
		f, e := os.Open(filename)
		if e != nil {
			panic(e)
		} else {
			lex = NewLexer(f)
		}
	}
	for callParse(filename, lex) {
	}
}

func callParse(filename string, lex *Lexer) (b bool) {
	defer func() {
		if e := recover(); e != nil {
			err := errors.New(fmt.Sprint(e))
			if logMode {
				fmt.Printf("%s:%d:%d: %s\n", filename, lex.Line()-1, lex.Column()+1, err.Error())
			} else {
				fmt.Printf("%s:%d:%d: %s\n", filename, lex.Line()+1, lex.Column()+1, err.Error())
			}
			b = true
		}
	}()
	if lex != nil {
		yyParse(lex)
	}
	return false
}

type luaFunc func(...interface{}) interface{}
type luaTable map[string]interface{}
type luaCoroutine struct{}

var (
	vals  map[string]interface{}
	funcs map[string]luaFunc
)

func init() {
	vals = map[string]interface{}{
		"_VERSION": "Lua 5.3 (BETA) ddosakura",
	}
	funcs = map[string]luaFunc{
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
		"type": func(args ...interface{}) interface{} {
			if len(args) == 0 {
				panic("bad argument #1 to 'type' (value expected)")
			}
			return valType(args[0])
		},
	}
}

func valType(a interface{}) string {
	switch a.(type) {
	case nil:
		return "nil"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case luaFunc:
		return "function"
	case luaTable:
		return "table"
	case luaCoroutine:
		return "thread"
	default:
		// return "userdata"
		return ""
	}
}

func unquote(s string) string {
	a, e := strconv.Unquote(`"` + s + `"`)
	if e != nil {
		panic(e)
	}
	return a
}
