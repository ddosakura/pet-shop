package main

import (
	"io/ioutil"
	"log"

	"github.com/kr/pretty"
	"github.com/llir/llvm/asm"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/types"
)

func main() {
	m, err := asm.ParseFile("main.ll")
	if err != nil {
		log.Fatalf("%+v", err)
	}
	p := ir.NewParam("", types.NewPointer(types.I8))
	f := m.NewFunc("xprintf", types.I32, p)
	f.Sig.Variadic = true
	ioutil.WriteFile("main2.ll", []byte(m.String()), 0644)
	pretty.Println(m.Funcs)
}
