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
    // fmt.Printf("L STR [%s]\n", lval.s)
    return STR
}
/[0-9]*(\.[0-9]*)?/ {
    var e error
    lval.n, e = strconv.ParseFloat(yylex.Text(), 64)
    if e != nil {
        panic(e)
    }
    nChars += len(yylex.Text())
    // fmt.Printf("L NUM [%s]\n", yylex.Text())
    return NUM
}
/[a-zA-Z_][a-zA-Z0-9_]*/ {
    lval.s = yylex.Text()
    nChars += len(yylex.Text())
    // fmt.Printf("L VAL [%s]\n", yylex.Text())
    return VAL
}

/--[^\n]*/ {
    /* one-line comments */
    lval.s = yylex.Text()[2:]
    // println("L ONE-LINE COMMENT", yylex.Text())
    return COMMENT
}
/--\[\[.*\]\]/ {
    /* multi-line comments */
    nLines += strings.Count(yylex.Text(), "\n")
    tmp := strings.Split(yylex.Text(), "\n")
    nChars=len(tmp[len(tmp)-1])
    lval.s = yylex.Text()[4:len(yylex.Text())-2]
    // println("L MULTI-LINE COMMENT", yylex.Text())
    return COMMENT
}

/[ \t]/ {
    /* Skip blanks and tabs. */
    nChars += len(yylex.Text())
}
/\n/ {
    nLines++
    nChars=0
    // print("> ")
    return int('\n')
}
/./ {
    nChars++
    // fmt.Printf("L CHAR[%s]\n", yylex.Text())
    return int(yylex.Text()[0])
}
//

package main
import("strconv"/*;"fmt"*/)
