package main

import (
	"fmt"
	"io/ioutil"

	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/types"
)

func main() {
	VOID := types.Void
	INT := types.I64

	TMP := constant.NewInt(INT, 2)

	m := ir.NewModule()
	m.SourceFilename = "main.src"

	ip := m.NewFunc("ip", INT)
	ipb := ip.NewBlock("input")
	ipb.NewRet(TMP)

	op := m.NewFunc("op", VOID, ir.NewParam("x", INT))
	opb := op.NewBlock("output")
	opb.NewUnreachable()

	add := m.NewFunc("add", INT, ir.NewParam("x", INT), ir.NewParam("y", INT))
	addb := add.NewBlock("add")
	tmp := addb.NewAdd(TMP, TMP)
	addb.NewRet(tmp)

	main := m.NewFunc("main", INT)
	mainb := main.NewBlock("main")
	a := mainb.NewCall(ip)
	b := mainb.NewCall(ip)
	c := mainb.NewCall(add, a, b)
	mainb.NewCall(op, c)
	mainb.NewRet(constant.NewInt(INT, 0))

	fmt.Println(m)

	ioutil.WriteFile("main.ll", []byte(m.String()), 0644)
}
