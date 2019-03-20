package main

import (
	"log"
	"os"

	"github.com/kr/pretty"
	"github.com/llir/llvm/asm"
	"github.com/llir/llvm/ir"
)

func main() {
	// Parse the LLVM IR assembly file `foo.ll`.
	m, err := asm.ParseFile(os.Args[1])
	if err != nil {
		log.Fatalf("%+v", err)
	}
	// Pretty-print the data types of the parsed LLVM IR module.
	pretty.Println(m.String())

	funcs := make(map[string]*ir.Func, len(m.Funcs))
	for _, v := range m.Funcs {
		funcs[v.Name()] = v
	}
	pretty.Println(funcs)
}
