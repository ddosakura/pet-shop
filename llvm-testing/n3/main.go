package main

import (
	"io/ioutil"

	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/enum"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

var (
	m    = ir.NewModule()
	zero = constant.NewInt(types.I32, 0)
	num0 = constant.NewInt(types.I64, 0)

	n3   = constant.NewInt(types.I32, 3)
	n4   = constant.NewInt(types.I32, 4)
	n6   = constant.NewInt(types.I32, 6)
	n7   = constant.NewInt(types.I32, 7)
	n9   = constant.NewInt(types.I32, 9)
	n100 = constant.NewInt(types.I32, 100)

	num3   = constant.NewInt(types.I64, 3)
	num4   = constant.NewInt(types.I64, 4)
	num6   = constant.NewInt(types.I64, 6)
	num7   = constant.NewInt(types.I64, 7)
	num9   = constant.NewInt(types.I64, 9)
	num100 = constant.NewInt(types.I64, 100)

	str = constant.NewCharArray([]byte{
		'%', 'l', 'l', 'd', '\n', 0,
	})
	strPtr = constant.NewGetElementPtr(m.NewGlobalDef(".str", str), zero, zero)

	pf = m.NewFunc("printf", types.I32, ir.NewParam("", types.I8Ptr))

	anon    = m.NewTypeDef("class.anon", types.NewStruct(types.I64))
	anonPtr = types.NewPointer(anon)
)

func main() {
	mainF := m.NewFunc("main", types.I32)
	b := mainF.NewBlock("")
	partMain(b)
	b.NewRet(zero)

	ioutil.WriteFile("main.ll", []byte(m.String()), 0644)
	// pretty.Println(m)
}

func init() {
	// strPtr.InBounds = true
	pf.Sig.Variadic = true
	partBar()
	partFoo()
}

func p(b *ir.Block, v value.Value) {
	b.NewCall(pf, strPtr, v)
}

var (
	barPA = ir.NewParam("", anonPtr)
	barPB = ir.NewParam("", types.I64)
	barF  = m.NewFunc("bar", types.I64, barPA, barPB)
)

func partBar() {
	barF.Visibility = enum.VisibilityHidden
	b := barF.NewBlock("")

	// ctx.a
	v3 := b.NewAlloca(anonPtr)
	b.NewStore(barPA, v3)
	// param.b
	v4 := b.NewAlloca(types.I64)
	b.NewStore(barPB, v4)

	// a
	v5 := b.NewLoad(v3)
	v6 := b.NewGetElementPtr(v5, zero, zero)
	v7 := b.NewLoad(v6)
	// b
	v8 := b.NewLoad(v4)
	// +
	vRet := b.NewAdd(v7, v8)

	b.NewRet(vRet)
}

var (
	fooP = ir.NewParam("", types.I64)
	fooF = m.NewFunc("foo", types.I64, fooP)
)

func partFoo() {
	b := fooF.NewBlock("")

	// param.a
	v3 := b.NewAlloca(types.I64)
	b.NewStore(fooP, v3)
	// local.b
	v2 := b.NewAlloca(anon)
	v4 := b.NewGetElementPtr(v2, zero, zero)

	// a
	v5 := b.NewLoad(v3)
	b.NewStore(v5, v4)
	// b
	v6 := b.NewGetElementPtr(v2, zero, zero)
	v7 := b.NewLoad(v6)

	b.NewRet(v7)
}

func partMain(b *ir.Block) {
	// num.0
	//v1 := b.NewAlloca(types.I64)
	//b.NewStore(num0, v1)
	partMainA(b)
	partMainB(b)
}

func partMainA(b *ir.Block) {
	// foo(4)
	v4 := b.NewCall(fooF, num4)
	// v2 = { v4 }
	v2 := b.NewAlloca(anon)
	v5 := b.NewGetElementPtr(v2, zero, zero)
	b.NewStore(v4, v5)
	v6 := b.NewCall(barF, v2, num6)
	// print
	p(b, v6)
}

func partMainB(b *ir.Block) {
	// foo(3)
	v8 := b.NewCall(fooF, num3)
	// v3 = { v8 }
	v3 := b.NewAlloca(anon)
	v9 := b.NewGetElementPtr(v3, zero, zero)
	b.NewStore(v8, v9)
	v10 := b.NewCall(barF, v3, num7)
	// print
	p(b, v10)
}
