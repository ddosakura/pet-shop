build: main.cpp
	clang -emit-llvm -S main.cpp -o main.ll

see: main.go main.ll
	go run main.go
