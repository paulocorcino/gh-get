# ast-grep

ast-grep reads code as a syntax tree. `npx -y -p @ast-grep/cli@0.51.1 ast-grep` (below, `ast-grep`) runs it at the version the repo pins. It sees syntax only: no types, no imports, nothing across files. A match is a candidate; the compiler and the linters decide what it means. The repo has no ast-grep rule and no gate step for it yet.

## Look before you read

Before reading a whole source file, outline it, then read only the range you need:

    ast-grep outline main.go                       # items and their members, with line numbers
    ast-grep outline internal/fetch --items exports  # in Go it lists every function, not only exported ones
    ast-grep outline main.go --items imports       # what a file depends on

`outline` is alpha: re-check it at a version bump.

## Search by shape

Search with ast-grep when the question is about the shape of code: every call of a function with a literal argument, every `if err != nil` with an empty body, every implementation of a method. Search with grep when it is about text: a name, a message, a comment, a file.

    ast-grep run -p '<pattern>' -l go <paths>
    ast-grep run -p '<pattern>' -l go <paths> --json=stream   # one match per line, to count

The pattern is Go code: `$X` matches one node, `$$$ARGS` any number of them.

A pattern that matches nothing is not proof the code is absent:

- A call to a package function is parsed as a type conversion when it stands alone: `os.Exit($C)` matches nothing. Wrap it in a function and select the call: `-p 'func _() { os.Exit($C) }' --selector call_expression`. A method call (`$X.Close()`) matches as written.
- `--debug-query=pattern` prints how ast-grep parsed the pattern; test it on a snippet you know contains the code.

## Rewrite

One shape changed at many sites:

    ast-grep run -p '<old>' -r '<new>' -l go <paths>      # prints the diff, changes nothing
    ast-grep run -p '<old>' -r '<new>' -l go <paths> -U   # applies it

Read the diff before `-U`. The rewrite drops the line breaks of the code it replaces: run `golangci-lint fmt` after it. A rewrite goes in a commit of its own, with the command in its message, and `scripts/check.sh` passes after it.

For rule syntax, see `https://ast-grep.github.io/llms.txt` (an index; fetch the one `.md` page needed) and the `ast-grep` agent skill (`ast-grep/agent-skill`).
