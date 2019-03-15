.PHONY: build
build:
	nex lua.l
	../goyacc -o=lua.yacc.go lua.y
	go fmt
	go build

.PHONY: clean
clean:
	-rm *.output *.yacc.go *.nn.go

.PHONY: g
g: build clean
