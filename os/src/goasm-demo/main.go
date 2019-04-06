package main

import "fmt"

/*
type stringStruct struct {
    str unsafe.Pointer  //指定底层的byte数组
    len int             //字符串长度
}
*/

func echo(msg string) int64 {
	fmt.Println(msg)
	return int64(len(msg))
}

// Hello World
func Hello() int64

func main() {
	Hello()
}
