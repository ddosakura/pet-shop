package main

import (
	"io/ioutil"

	"github.com/kr/pretty"
	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/types"
)

func main() {
	zero := constant.NewInt(types.I32, 0)

	m := ir.NewModule()
	str := constant.NewCharArray([]byte{
		'%', 'l', 'l', 'd', '\n', 0,
	})
	strPtr := constant.NewGetElementPtr(m.NewGlobalDef(".str", str), zero, zero)
	// strPtr.InBounds = true

	pf := m.NewFunc("printf", types.I32, ir.NewParam("", types.I8Ptr))
	pf.Sig.Variadic = true

	f := m.NewFunc("main", types.I32)
	b := f.NewBlock("")
	b.NewCall(pf, strPtr, constant.NewInt(types.I64, 128))
	b.NewRet(zero)

	ioutil.WriteFile("main.ll", []byte(m.String()), 0644)
	pretty.Println(m)
}
