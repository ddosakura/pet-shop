/[ \t]/ {
    /* Skip blanks and tabs. */
    nChars += len(yylex.Text())
}
/[0-9]*(\.[0-9]*)?/ {
    var e error
    lval.n, e = strconv.ParseFloat(yylex.Text(), 64)
    if e != nil {
        panic(e)
    }
    // println("NUM", yylex.Text())
    nChars += len(yylex.Text())
    return NUM
}
/\+|-/ {
    lval.op = yylex.Text()
    nChars += len(yylex.Text())
    return OP2
}
/\*|\// {
    lval.op = yylex.Text()
    nChars += len(yylex.Text())
    return OP1
}
/\n/ {
    nLines++
    nChars=0
    return ANS
}
/./ {
    nChars++
}
//
package main
import("strconv")
