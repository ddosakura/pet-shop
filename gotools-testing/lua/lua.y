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

expr: data {
        // println("Y expr | data")
        $$.v = $1.v
    } | VAL {
        $$.v = vals[$1.s]
    } | call {
        // println("Y expr | call")
        $$.v = $1.v
    };

call: VAL '(' args ')' {
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

data: NIL {
        $$.v = nil
    } | BOOL {
        $$.v = $1.b
    } | STR {
        $$.v = $1.s
    } | NUM {
        $$.v = $1.n
    }
%%

func emit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stdout,format, a...);
	fmt.Fprintln(os.Stdout,"")
}
func die(format string, a ...interface{}) {
    panic(fmt.Sprintf(format, a...))
}
