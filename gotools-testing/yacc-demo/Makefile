.PHONY: build
build:
	nex demo.l
	../goyacc -o=demo.yacc.go demo.y
	go fmt
	go build

.PHONY: clean
clean:
	-rm *.output *.yacc.go *.nn.go

.PHONY: g
g: build clean
