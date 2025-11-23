package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"ashn.dev/mellifera"
)

func dumpTokensSource(ctx *mellifera.Context, source string, location *mellifera.SourceLocation) error {
	tokens := ctx.NewVector(nil)
	lexer := mellifera.NewLexer(ctx, source, location)

	token, err := lexer.NextToken()
	if err != nil {
		return err
	}
	for token.Kind != mellifera.TOKEN_EOF {
		tokens.Push(token.IntoValue(ctx))
		token, err = lexer.NextToken()
		if err != nil {
			return err
		}
	}

	var sb strings.Builder
	encoder := mellifera.NewCombEncoder(&sb, mellifera.Ptr("    "))
	err = tokens.CombEncode(encoder)
	if err != nil {
		return err
	}
	fmt.Println(sb.String())
	return nil
}

func dumpTokensFile(ctx *mellifera.Context, path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	source := string(bytes)
	return dumpTokensSource(ctx, source, &mellifera.SourceLocation{path, 1})
}

func usage(w io.Writer) {
	program := os.Args[0]
	fmt.Fprintf(w, `usage:
  %s FILE [ARGS...]
  %s [-c|--command] COMMAND [ARGS...]

options:
  -c, --command     Execute the provided command.
  --dump-tokens     Dump a comb-encoded a vector of lexed tokens to stdout.
  -h, --help        Display this help text and exit.
`, program, program)
}

func main() {
	reCommand := regexp.MustCompile(`^-+c(?:ommand)?(?:=(.*))?$`)
	reDumpTokens := regexp.MustCompile(`^-+dump-tokens$`)
	reHelp := regexp.MustCompile(`^-+h(?:elp)?(?:=(.*))?$`)

	verbatim := false
	var cmds *string
	var file *string
	var argv []string
	dumpTokens := false
	argi := 1
	for argi < len(os.Args) {
		arg := os.Args[argi]

		positional := func() {
			if cmds == nil && file == nil {
				file = &arg
				argv = append(argv, arg)
				verbatim = true
				argi += 1
				return
			}

			argv = append(argv, arg)
			argi += 1
		}

		if verbatim {
			positional()
			continue
		}

		// Remaining args are processed verbatim.
		if arg == "--" {
			verbatim = true
			argi += 1
			continue
		}

		// -c, -command
		if m := reCommand.FindStringSubmatch(arg); m != nil {
			// -c='println("hello world");'
			if m[1] != "" {
				cmds = &m[1]
				argv = append([]string{os.Args[0]}, os.Args[argi+1:]...)
				break
			}

			// -c 'println("hello world");'
			if argi+1 < len(os.Args) {
				cmds = &os.Args[argi+1]
				argv = append([]string{os.Args[0]}, os.Args[argi+2:]...)
				break
			}

			fmt.Fprintf(os.Stderr, "error: expected command argument\n")
			usage(os.Stderr)
			os.Exit(1)
		}

		// -dump-tokens
		if m := reDumpTokens.FindStringSubmatch(arg); m != nil {
			dumpTokens = true
			argi += 1
			continue
		}

		// -h, -help
		if m := reHelp.FindStringSubmatch(arg); m != nil {
			usage(os.Stdout)
			os.Exit(0)
		}

		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "error: unknown flag %s\n", arg)
			usage(os.Stderr)
			os.Exit(1)
		}

		positional()
	}

	/*
		fmt.Printf("verbatim is %v\n", verbatim)
		fmt.Printf("cmds is %v\n", cmds)
		if cmds != nil {
			fmt.Printf("\t*cmds is `%v`\n", *cmds)
		}
		fmt.Printf("file is %v\n", file)
		if file != nil {
			fmt.Printf("\t*file is `%v`\n", *file)
		}
		fmt.Printf("argv is %v\n", argv)
		fmt.Printf("dumpTokens is %v\n", dumpTokens)
	*/

	var err error
	ctx := mellifera.NewContext()
	if cmds != nil || file != nil {
		if cmds != nil && dumpTokens {
			err = dumpTokensSource(&ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if cmds != nil {
			err = fmt.Errorf("error: eval command not implemented\n")
		} else if file != nil && dumpTokens {
			err = dumpTokensFile(&ctx, *file)
		} else if file != nil {
			err = fmt.Errorf("error: eval file not implemented\n")
		} else {
			err = fmt.Errorf("unreachable\n")
		}
	} else if dumpTokens {
		fmt.Fprintf(os.Stderr, "error: requested token dump without a command or file path\n")
		os.Exit(1)
	} else {
		fmt.Fprintf(os.Stderr, "error: REPL not implemented\n")
		os.Exit(1)
	}

	if err != nil {
		if parseError, ok := err.(mellifera.ParseError); ok && parseError.Location != nil {
			fmt.Fprintf(os.Stderr, "[%v:%v] %v\n", parseError.Location.File, parseError.Location.Line, err)
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(1)
	}
}
