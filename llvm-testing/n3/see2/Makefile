main.ll: main.cpp
	clang -emit-llvm -S main.cpp -o main.ll
main.bc: main.ll
	llvm-as main.ll -o main.bc

see: main.go main.ll
	go run main.go
