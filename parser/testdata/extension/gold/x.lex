module "module"
[ident] "x"
{ "{"
prefix "prefix"
[string] "\"x\""
; ";"
extension "extension"
[ident] "ext"
{ "{"
description "descriptio"...
[string] "\"x\""
; ";"
argument "argument"
[string] "\"f\""
{ "{"
yin-element "yin-elemen"...
true "true"
; ";"
} "}"
argument "argument"
[string] "\"g\""
; ";"
} "}"
extension "extension"
[ident] "ext2"
{ "{"
} "}"
description "descriptio"...
[string] "\"x\""
{ "{"
[unknown] "x:ext"
[number] "5"
; ";"
} "}"
leaf "leaf"
[ident] "x"
{ "{"
type "type"
[ident] "string"
; ";"
} "}"
[unknown] "x:ext"
[string] "\"a\""
[string] "\"{\""
[number] "2"
[string] "\"}\""
; ";"
} "}"
