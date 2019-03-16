/and/ { return AND }
/or/ { return OR }
/not/ { return NOT }

/if/ { return IF }
/then/ { return THEN }
/elseif/ { return ELIF }
/else/ { return ELSE }

/do/ { return DO }
/end/ { return END }
/while/ { return WHILE }
/for/ { return FOR }
/repeat/ { return REPEAT }
/until/ { return UNTIL }
/break/ { return BREAK }

/function/ { return FUNC }
/return/ { return RET }

/in/ { return IN }
/local/ { return LOCAL }

/nil/ { return NIL }
/true/ { lval.b = true; return BOOL }
/false/ { lval.b = false; return BOOL }

/("([^"\\]|\\.)*")|('([^'\\]|\\.)*')/ {
    lval.s = unquote(yylex.Text()[1:len(yylex.Text())-1])
	// log.Printf("L STR [%s]\n", lval.s)
    return STR
}
/[0-9]*(\.[0-9]*)?/ {
    var e error
    lval.n, e = strconv.ParseFloat(yylex.Text(), 64)
    if e != nil {
        panic(e)
    }
    // log.Printf("L NUM [%s]\n", yylex.Text())
    return NUM
}
/[a-zA-Z_][a-zA-Z0-9_]*/ {
    lval.s = yylex.Text()
    // log.Printf("L VAL [%s]\n", yylex.Text())
    return VAL
}

/--\[\[[^\]]*(\][^\]]+)*\]\]/ {
    /* multi-line comments */
    lval.s = yylex.Text()[4:len(yylex.Text())-2]
    // log.Printf("L MULTI-LINE COMMENT {{%s}}\n", lval.s)
    return COMMENT
}
/--\[=\[[^\]]*(\][^=]+)*\]=\]/ {
    /* multi-line comments */
    lval.s = yylex.Text()[5:len(yylex.Text())-3]
    // log.Printf("L MULTI-LINE COMMENT {{%s}}\n", lval.s)
    return COMMENT
}
/--[^\n]*/ {
    /* one-line comments */
    lval.s = yylex.Text()[2:]
    // log.Printf("L ONE-LINE COMMENT {{%s}}\n", yylex.Text())
    return COMMENT
}

/[ \t]/ {
    /* Skip blanks and tabs. */
    // log.Printf("L <space/tab>\n")
}
/\n/ {
    // log.Printf("L <enter>\n")
    // println("L <enter>")
    // return int('\n')
    if logMode {
        go func() {
            time.Sleep(10 * time.Millisecond)
            print("> ")
        }()
    }
}
/./ {
    return int(yylex.Text()[0])
}
//

package main
import("strconv";"time"/*;"log"*/)
