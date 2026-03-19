package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	return dumpTokensSource(ctx, string(bytes), &mellifera.SourceLocation{path, 1})
}

func dumpAstSource(ctx *mellifera.Context, source string, location *mellifera.SourceLocation) error {
	lexer := mellifera.NewLexer(ctx, source, location)
	parser, err := mellifera.NewParser(&lexer)
	if err != nil {
		return err
	}
	program, err := parser.ParseProgram()
	if err != nil {
		return err
	}

	var sb strings.Builder
	encoder := mellifera.NewCombEncoder(&sb, mellifera.Ptr("    "))
	err = program.IntoValue(ctx).CombEncode(encoder)
	if err != nil {
		return err
	}
	fmt.Println(sb.String())
	return nil
}

func dumpAstFile(ctx *mellifera.Context, path string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return dumpAstSource(ctx, string(bytes), &mellifera.SourceLocation{path, 1})
}

func evalSource(ctx *mellifera.Context, source string, location *mellifera.SourceLocation) (mellifera.Value, error) {
	lexer := mellifera.NewLexer(ctx, source, location)
	parser, err := mellifera.NewParser(&lexer)
	if err != nil {
		return nil, err
	}
	program, err := parser.ParseProgram()
	if err != nil {
		return nil, err
	}
	return program.Eval(ctx, &ctx.BaseEnvironment)
}

func evalFile(ctx *mellifera.Context, path string) (mellifera.Value, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return evalSource(ctx, string(bytes), &mellifera.SourceLocation{path, 1})
}

func mfenv(w io.Writer) {
	fmt.Fprintf(w, "MELLIFERA_HOME=%s\n", os.Getenv("MELLIFERA_HOME"))
	fmt.Fprintf(w, "MELLIFERA_SEARCH_PATH=%s\n", os.Getenv("MELLIFERA_SEARCH_PATH"))
}

func usage(w io.Writer) {
	program := os.Args[0]
	fmt.Fprintf(w, `usage:
  %s FILE [ARGS...]
  %s [-c|--command] COMMAND [ARGS...]

options:
  -c, --command     Execute the provided command.
  --dump-tokens     Dump a comb-encoded vector of lexed tokens to stdout.
  --dump-ast        Dump a comb-encoded abstract syntax tree to stdout.
  -e, --env         Display the mellifera environment and exit.
  -h, --help        Display this help text and exit.
`, program, program)
}

func main() {
	reCommand := regexp.MustCompile(`^-+c(?:ommand)?(?:=(.*))?$`)
	reDumpTokens := regexp.MustCompile(`^-+dump-tokens$`)
	reDumpAst := regexp.MustCompile(`^-+dump-ast$`)
	reEnv := regexp.MustCompile(`^-+e(?:nv)?$`)
	reHelp := regexp.MustCompile(`^-+h(?:elp)?$`)

	envMELLIFERA_HOME, ok := os.LookupEnv("MELLIFERA_HOME")
	if !ok {
		// $MELLIFERA_HOME/bin/mf
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
		// $MELLIFERA_HOME/bin
		bin := filepath.Dir(exe)
		// $MELLIFERA_HOME
		envMELLIFERA_HOME = filepath.Dir(bin)
		if err = os.Setenv("MELLIFERA_HOME", envMELLIFERA_HOME); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
	}
	envMELLIFERA_SEARCH_PATH, ok := os.LookupEnv("MELLIFERA_SEARCH_PATH")
	if !ok {
		envMELLIFERA_SEARCH_PATH = fmt.Sprintf("%s/lib", envMELLIFERA_HOME)
		if err := os.Setenv("MELLIFERA_SEARCH_PATH", envMELLIFERA_SEARCH_PATH); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
	}

	verbatim := false
	var cmds *string
	var file *string
	var argv []string
	dumpTokens := false
	dumpAst := false
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

		// -dump-ast
		if m := reDumpAst.FindStringSubmatch(arg); m != nil {
			dumpAst = true
			argi += 1
			continue
		}

		// -e, -env
		if m := reEnv.FindStringSubmatch(arg); m != nil {
			mfenv(os.Stdout)
			os.Exit(0)
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

	var err error
	ctx := mellifera.NewContext()

	argvIntoValue := func() mellifera.Value {
		result := ctx.NewVector(nil)
		for _, arg := range argv {
			result.Push(ctx.NewString(arg))
		}
		return result
	}
	ctx.BaseEnvironment.Let("argv", argvIntoValue())

	var path string
	if file != nil {
		path, err = filepath.Abs(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
	} else {
		path, err = filepath.Abs(os.Args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
	}
	module, err := ctx.BaseEnvironment.Get("module")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}
	module.(*mellifera.Map).Insert(ctx.NewString("path"), ctx.NewString(path))
	module.(*mellifera.Map).Insert(ctx.NewString("file"), ctx.NewString(filepath.Base(path)))
	module.(*mellifera.Map).Insert(ctx.NewString("directory"), ctx.NewString(filepath.Dir(path)))

	if dumpTokens && dumpAst {
		fmt.Fprintf(os.Stderr, "error: requested token dump and AST dump which are mutually exclusive\n")
		os.Exit(1)
	} else if cmds != nil || file != nil {
		if cmds != nil && dumpTokens {
			err = dumpTokensSource(&ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if cmds != nil && dumpAst {
			err = dumpAstSource(&ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if cmds != nil {
			_, err = evalSource(&ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if file != nil && dumpTokens {
			err = dumpTokensFile(&ctx, *file)
		} else if file != nil && dumpAst {
			err = dumpAstFile(&ctx, *file)
		} else if file != nil {
			_, err = evalFile(&ctx, *file)
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
		if e, ok := err.(mellifera.ParseError); ok && e.Location != nil {
			fmt.Fprintf(os.Stderr, "[%v, line %v] error: %v\n", e.Location.File, e.Location.Line, err)
		} else if e, ok := err.(mellifera.Error); ok {
			if e.Location != nil {
				fmt.Fprintf(os.Stderr, "[%v, line %v] error: %v\n", e.Location.File, e.Location.Line, err)
			} else {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			for _, element := range e.Trace {
				s := fmt.Sprintf("...within %v", element.FuncName)
				if element.Location != nil {
					s += fmt.Sprintf(" called from %s, line %v", element.Location.File, element.Location.Line)
				}
				fmt.Fprintf(os.Stderr, "%s\n", s)
			}
		} else {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(1)
	}
}
