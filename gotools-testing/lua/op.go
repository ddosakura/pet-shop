package main

import (
	"math"
)

// a^b
func opPow(a, b interface{}) float64 {
	return math.Pow(a.(float64), b.(float64))
}

// ---

// not a
func opNot(a interface{}) bool {
	return !a.(bool)
}

// -a
func opNegative(a interface{}) float64 {
	return -a.(float64)
}

// #a
func opLen(a interface{}) float64 {
	// TODO: table type
	l := len(a.(string))
	return float64(l)
}

// ---

// a*b
func opMultiply(a, b interface{}) float64 {
	return a.(float64) * b.(float64)
}

// a/b
func opDevide(a, b interface{}) float64 {
	return a.(float64) / b.(float64)
}

// a%b
func opMod(a, b interface{}) float64 {
	return math.Mod(a.(float64), b.(float64))
}

// ---

// a+b
func opAdd(a, b interface{}) float64 {
	return a.(float64) + b.(float64)
}

// a-b
func opMinus(a, b interface{}) float64 {
	return a.(float64) - b.(float64)
}

// ---

// "a".."b"
func opStrAppend(a, b interface{}) string {
	return a.(string) + b.(string)
}

// ---

// a<b
func opLT(a, b interface{}) bool {
	return a.(float64) < b.(float64)
}

// a<=b
func opLE(a, b interface{}) bool {
	return opLT(a, b) || opEQ(a, b)
}

// a>b
func opGT(a, b interface{}) bool {
	return a.(float64) > b.(float64)
}

// a>=b
func opGE(a, b interface{}) bool {
	return opGT(a, b) || opEQ(a, b)
}

// a==b
func opEQ(a, b interface{}) bool {
	if valType(a) != valType(b) {
		panic("attempt to compare " + valType(a) + " with " + valType(b))
	}
	switch a.(type) {
	case float64:
		return a.(float64) == b.(float64)
	case bool:
		return a.(bool) == b.(bool)
	case string:
		return a.(string) == b.(string)
	case nil:
		return b == nil
	}
	panic("attempt to compare two " + valType(a) + " values")
}

// a~=b
func opNE(a, b interface{}) bool {
	return !opEQ(a, b)
}

// ---

// a and b
func opAnd(a, b interface{}) bool {
	return a.(bool) && b.(bool)
}

// ---

// a or b
func opOr(a, b interface{}) bool {
	return a.(bool) || b.(bool)
}
