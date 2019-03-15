package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTmp(t *testing.T) {
	txt := "sd\ns\na\nsdd"
	fmt.Println(strings.Count(txt, "\n"))
	tmp := strings.Split(txt, "\n")
	txt2 := "asd\\\\\\a\\n"
	println("\\\\(.)", txt2)
	r := regexp.MustCompile("\\\\(.)")
	o := r.ReplaceAllString(txt2, "$1")
	println(len(tmp[len(tmp)-1]), strings.ReplaceAll("asd\\\\", "\\\\", "\\"), o)
	a, e := strconv.Unquote(`"` + txt2 + `"`)
	println(a, e)
}
