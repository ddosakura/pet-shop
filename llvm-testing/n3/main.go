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

	n0   = constant.NewInt(types.I32, 0)
	n1   = constant.NewInt(types.I32, 1)
	n3   = constant.NewInt(types.I32, 3)
	n4   = constant.NewInt(types.I32, 4)
	n6   = constant.NewInt(types.I32, 6)
	n7   = constant.NewInt(types.I32, 7)
	n9   = constant.NewInt(types.I32, 9)
	n100 = constant.NewInt(types.I32, 100)

	num0   = constant.NewInt(types.I64, 0)
	num1   = constant.NewInt(types.I64, 1)
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

	anon    = m.NewTypeDef("class.anon", types.NewStruct(types.I64, types.I64))
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
	barP1 = ir.NewParam("", types.I64)
	barP2 = ir.NewParam("", types.I64)
	barF  = m.NewFunc("bar", types.I64, barPA, barP1, barP2)
)

func partBar() {
	barF.Visibility = enum.VisibilityHidden
	b := barF.NewBlock("")

	// param.b1
	v5 := b.NewAlloca(types.I64)
	b.NewStore(barP1, v5)
	// param.b2
	v6 := b.NewAlloca(types.I64)
	b.NewStore(barP2, v6)

	// ctx.a
	v4 := b.NewAlloca(anonPtr)
	b.NewStore(barPA, v4)
	v7 := b.NewLoad(v4)
	// a1
	v8 := b.NewGetElementPtr(v7, n0, n0)
	v9 := b.NewLoad(v8)
	// b1 += a1
	v10 := b.NewLoad(v5)
	v11 := b.NewAdd(v9, v10)
	// a2
	v12 := b.NewGetElementPtr(v7, n0, n1)
	v13 := b.NewLoad(v12)
	// b2 += a2
	v14 := b.NewLoad(v6)
	v15 := b.NewAdd(v13, v14)
	// *
	v16 := b.NewMul(v11, v15)

	b.NewRet(v16)
}

var (
	fooP1 = ir.NewParam("", types.I64)
	fooP2 = ir.NewParam("", types.I64)
	fooF  = m.NewFunc("foo", types.I128, fooP1, fooP2)
)

func partFoo() {
	b := fooF.NewBlock("")

	// v4 = param.a1
	v4 := b.NewAlloca(types.I64)
	b.NewStore(fooP1, v4)
	// v5 = param.a2
	v5 := b.NewAlloca(types.I64)
	b.NewStore(fooP2, v5)

	// v3 = { v4, v5 }
	v3 := b.NewAlloca(anon)
	v6 := b.NewGetElementPtr(v3, n0, n0)
	v7 := b.NewLoad(v4)
	b.NewStore(v7, v6)
	v8 := b.NewGetElementPtr(v3, n0, n1)
	v9 := b.NewLoad(v5)
	b.NewStore(v9, v8)

	v10 := b.NewBitCast(v3, types.I128Ptr)
	v11 := b.NewLoad(v10)

	b.NewRet(v11)
}

func partMain(b *ir.Block) {
	// foo(4, 3)
	v3 := b.NewCall(fooF, num4, num3)
	// v2 = { v3... }
	v2 := b.NewAlloca(anon)
	v4 := b.NewBitCast(v2, types.I128Ptr)
	b.NewStore(v3, v4)

	// bar(ctx.v2, 6, 7)
	v5 := b.NewCall(barF, v2, num6, num7)

	// print
	p(b, v5)
}
