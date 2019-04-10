package main

import (
	"io/ioutil"

	"github.com/llir/llvm/ir"
	"github.com/llir/llvm/ir/constant"
	"github.com/llir/llvm/ir/types"
	"github.com/llir/llvm/ir/value"
)

var (
	m = ir.NewModule()

	zero   = constant.NewInt(types.I32, 0)
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
	b.NewCall(fooF, num4)
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
	b := barF.NewBlock("")

	vA := b.NewAlloca(anonPtr)
	b.NewStore(barPA, vA)
	vB := b.NewAlloca(types.I64)
	b.NewStore(barPB, vB)

	vC := b.NewLoad(vA)
	vD := b.NewGetElementPtr(vC, zero, zero)

	vE := b.NewLoad(vD)
	vF := b.NewLoad(vB)
	vRet := b.NewAdd(vE, vF)

	b.NewRet(vRet)
}

var (
	fooP = ir.NewParam("", types.I64)
	fooF = m.NewFunc("foo", types.Void, fooP)
)

func partFoo() {
	b := fooF.NewBlock("")

	vA := b.NewAlloca(types.I64)
	b.NewStore(fooP, vA)

	// auto fun = [=](int b)->int {return a+b;};
	vS := b.NewAlloca(anon)
	vB := b.NewAlloca(types.I64)
	fooPtr := b.NewGetElementPtr(vS, zero, zero)
	fooTmp := b.NewLoad(vA)
	b.NewStore(fooTmp, fooPtr)

	// int c = fun(100);
	vC := b.NewCall(barF, vS, num6)
	b.NewStore(vC, vB)
	vD := b.NewLoad(vB)
	p(b, vD)

	b.NewRet(nil)
}
