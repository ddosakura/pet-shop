%{
package main

// import ("fmt";"os";"io/ioutil";"flag";"bufio")
%}

%union {
    n  float64
    op string
}

%token NUM
%token ANS
%token OP1
%token OP2

%%
expr: expr2 { println("ANS =", $1.n) } | expr ANS expr2 { println("ANS =", $3.n) };

expr2: expr1
    | expr2 OP2 expr1 {
        if $2.op == "+" {
            $$.n = $1.n + $3.n
        } else {
            $$.n = $1.n - $3.n
        }
    };

expr1: NUM {
        $$.n = $1.n}
    | expr1 OP1 NUM {
        if $2.op == "*" {
            $$.n = $1.n * $3.n
        } else {
            $$.n = $1.n / $3.n
        }
    };
%%
