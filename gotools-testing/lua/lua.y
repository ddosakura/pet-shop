%{
package main

import ("fmt";"os")
%}

%union {
    n    float64
    b    bool
    s    string

    v    interface{}
    args []interface{}
}

%token AND
%token OR
%token NOT

%token LT
%token LE
%token GT
%token GE
%token EQ
%token NE

%token StrAppend

%token IF
%token THEN
%token ELIF
%token ELSE

%token DO
%token END
%token WHILE
%token FOR
%token REPEAT
%token UNTIL
%token BREAK

%token FUNC
%token RET

%token IN
%token LOCAL

%token NIL
%token BOOL

%token STR
%token NUM
%token VAL

%token COMMENT

%%
prog: stat {
        // println("Y prog | stat")
    } | prog stat {
        // println("Y prog | prog stat")
    } | ;

stat: expr {
        // println("Y stat | expr")
    } | LOCAL VAL'=' expr {
        /* TODO: */
    } | VAL '=' expr {
        vals[$1.s] = $3.v
    } | COMMENT {
        // fmt.Printf("Y stat | COMMENT {{%s}}\n", $$.s)
    } | ';' {
        // println("Y stat | ;")
    };

expr: expr7 {
        $$.v = $1.v
    } | expr7 OR expr7 {
        $$.v = opOr($1.v, $3.v)
    };

expr7: expr6 {
        $$.v = $1.v
    } | expr6 AND expr6 {
        $$.v = opAnd($1.v, $3.v)
    };

expr6: expr5 {
        $$.v = $1.v
    } | expr5 LT expr5 {
        $$.v = opLT($1.v, $3.v)
    } | expr5 LE expr5 {
        $$.v = opLE($1.v, $3.v)
    } | expr5 GT expr5 {
        $$.v = opGT($1.v, $3.v)
    } | expr5 GE expr5 {
        $$.v = opGE($1.v, $3.v)
    } | expr5 EQ expr5 {
        $$.v = opEQ($1.v, $3.v)
    } | expr5 NE expr5 {
        $$.v = opNE($1.v, $3.v)
    };

expr5: expr4 {
        $$.v = $1.v
    } | expr4 StrAppend expr4 {
        $$.v = opStrAppend($1.v, $3.v)
    };

expr4: expr3 {
        $$.v = $1.v
    } | expr3 '+' expr3 {
        $$.v = opAdd($1.v, $3.v)
    } | expr3 '-' expr3 {
        $$.v = opMinus($1.v, $3.v)
    };

expr3: expr2 {
        $$.v = $1.v
    } | expr2 '*' expr2 {
        $$.v = opMultiply($1.v, $3.v)
    } | expr2 '/' expr2 {
        $$.v = opDevide($1.v, $3.v)
    } | expr2 '%' expr2 {
        $$.v = opMod($1.v, $3.v)
    };

expr2: expr1 {
        $$.v = $1.v
    } | NOT expr1 {
        $$.v = opNot($2.v)
    } | '-' expr1 {
        $$.v = opNegative($2.v)
    } | '#' expr0 {
        $$.v = opLen($2.v)
    };

expr1: expr0 {
        $$.v = $1.v
    } | expr0 '^' expr0 {
        $$.v = opPow($1.v, $3.v)
    };

expr0: data {
        $$.v = $1.v
    } | '(' data ')' {
        $$.v = $2.v
    };

data: NIL {
        $$.v = nil
    } | BOOL {
        $$.v = $1.b
    } | STR {
        $$.v = $1.s
    } | NUM {
        $$.v = $1.n
    } | VAL {
        $$.v = vals[$1.s]
    } | VAL '(' args ')' {
        fn := funcs[$1.s]
        if fn == nil {
            die("attempt to call a nil value (global '%s')", $1.s)
        }
        // println($1.s)
        // fmt.Printf("%#v\n", $3.args)
        $$.v = fn($3.args...)
    };

args: expr {
        $$.args = []interface{}{$1.v}
    } | args ',' expr {
        $$.args = append($1.args, $3.v)
        // fmt.Printf("%#v %#v %#v\n", $1.args, $3.v, $$.args)
    } | ;
%%

func emit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stdout,format, a...);
	fmt.Fprintln(os.Stdout,"")
}
func die(format string, a ...interface{}) {
    panic(fmt.Sprintf(format, a...))
}
