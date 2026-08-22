package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ashn.dev/mellifera"
)

func BuiltinExit(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("exit", []mellifera.Type{
		mellifera.TVal(mellifera.NUMBER),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		integer, err := mellifera.ValueAsSafeInteger(arguments[0])
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("expected integer exit code, received %v", arguments[0]))
		}
		os.Exit(int(integer))
		return ctx.NewNull(), nil
	})
}

func BuiltinImport(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("import", []mellifera.Type{
		mellifera.TVal(mellifera.STRING),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		target := arguments[0].(*mellifera.String)

		module, err := ctx.BaseEnvironment.Get("module")
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
		}
		moduleMap, ok := module.(*mellifera.Map)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("expected map module value, received %v", module.Typename()))
		}
		_, modulePathOk := moduleMap.Lookup(ctx.NewString("path"))
		_, moduleFileOk := moduleMap.Lookup(ctx.NewString("file"))
		moduleDirectory, moduleDirectoryOk := moduleMap.Lookup(ctx.NewString("directory"))
		if !modulePathOk || !moduleFileOk || !moduleDirectoryOk {
			return nil, mellifera.NewError(nil, ctx.NewStringf("expected module map to contain `path`, `file` and `directory` values, received %v", module))
		}
		moduleDirectoryString, ok := moduleDirectory.(*mellifera.String)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("expected string module directory value, received %v", moduleDirectory))
		}

		// Always restore module fields.
		defer (func() {
			_ = ctx.BaseEnvironment.Set("module", moduleMap)
		})()

		var result mellifera.Value = nil
		paths := []string{}
		if filepath.IsAbs(target.Data()) {
			paths = append(paths, target.Data()) // direct path
		} else {
			paths = append(paths, filepath.Join(moduleDirectoryString.Data(), target.Data())) // current module directory
			if search, ok := os.LookupEnv("MELLIFERA_SEARCH_PATH"); ok {
				for _, p := range strings.Split(search, ":") {
					paths = append(paths, filepath.Join(p, target.Data()))
				}
			}
		}
		for _, p := range paths {
			stat, err := os.Stat(p)
			if err != nil {
				continue
			}
			if stat.IsDir() {
				// If the path is a directory, such as in the case of a
				// library, load the entry point to the library and/or group of
				// files, using the name <directory>/lib.mf by convention.
				p = filepath.Join(p, "lib.mf")
			}
			absolute, err := filepath.Abs(p)
			if err != nil {
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}

			if cached, ok := ctx.GetModule(absolute); ok {
				return cached, nil
			}

			importModuleMap := ctx.NewMapOrPanic([]mellifera.MapPair{
				{ctx.NewString("path"), ctx.NewString(absolute)},
				{ctx.NewString("file"), ctx.NewString(filepath.Base(absolute))},
				{ctx.NewString("directory"), ctx.NewString(filepath.Dir(absolute))},
			}).Freeze()
			if err := ctx.BaseEnvironment.Set("module", importModuleMap); err != nil {
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}

			bytes, err := os.ReadFile(absolute)
			if err != nil {
				continue
			}
			source := string(bytes)

			lexer := mellifera.NewLexer(ctx, source, &mellifera.SourceLocation{p, 1})
			parser, err := mellifera.NewParser(&lexer)
			if err != nil {
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}
			program, err := parser.ParseProgram()
			if err != nil {
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}
			result, err = program.Eval(ctx, mellifera.NewEnvironment(ctx.BaseEnvironment))
			if err != nil {
				if e, ok := err.(mellifera.Error); ok {
					return nil, e
				}
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}
			ctx.SetModule(absolute, result) // cache import
			break
		}

		if result == nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("module %v not found", target))
		}
		return result, nil
	})
}

func BuiltinInput(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("input", []mellifera.Type{}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		data, err := io.ReadAll(ctx.Stdin)
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
		}
		return ctx.NewString(string(data)), nil
	})
}

func BuiltinInputln(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("inputln", []mellifera.Type{}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		data := []byte{}
		buf := make([]byte, 1)
		for {
			nbytes, err := ctx.Stdin.Read(buf)
			if nbytes != 0 {
				if buf[0] == '\n' {
					break
				}
				data = append(data, buf[0])
			}
			if err != nil {
				if err == io.EOF {
					if len(data) == 0 {
						// The end of input was reached before any line data
						// was read. Produce null so that the end of input is
						// distinguishable from an empty line.
						return ctx.NewNull(), nil
					}
					break
				}
				return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
			}
		}
		return ctx.NewString(string(data)), nil
	})
}

func BuiltinFsRead(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("fs::read", []mellifera.Type{
		mellifera.TVal(mellifera.STRING),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		path := arguments[0].(*mellifera.String)

		data, err := os.ReadFile(path.Data())
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				if errors.Is(pathErr.Err, os.ErrNotExist) {
					return nil, mellifera.NewError(nil, ctx.NewStringf("failed to read file %v (file not found)", path))
				}
			}
			return nil, mellifera.NewError(nil, ctx.NewStringf("failed to read file %v (%s)", path, err.Error()))
		}
		return ctx.NewString(string(data)), nil
	})
}

func BuiltinFsWrite(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("fs::write", []mellifera.Type{
		mellifera.TVal(mellifera.STRING),
		mellifera.TVal(mellifera.STRING),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		path := arguments[0].(*mellifera.String)
		data := arguments[1].(*mellifera.String)

		f, err := os.OpenFile(path.Data(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("failed write to file %v (%s)", path, err.Error()))
		}
		defer f.Close()
		if _, err := f.Write([]byte(data.Data())); err != nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("failed write to file %v (%s)", path, err.Error()))
		}
		return ctx.NewNull(), nil
	})
}

func BuiltinFsAppend(ctx *mellifera.Context) mellifera.Value {
	return ctx.NewBuiltin("fs::append", []mellifera.Type{
		mellifera.TVal(mellifera.STRING),
		mellifera.TVal(mellifera.STRING),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		path := arguments[0].(*mellifera.String)
		data := arguments[1].(*mellifera.String)

		f, err := os.OpenFile(path.Data(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("failed append to file %v (%s)", path, err.Error()))
		}
		defer f.Close()
		if _, err := f.Write([]byte(data.Data())); err != nil {
			return nil, mellifera.NewError(nil, ctx.NewStringf("failed append to file %v (%s)", path, err.Error()))
		}
		return ctx.NewNull(), nil
	})
}

func dumpTokensSource(ctx *mellifera.Context, source string, location *mellifera.SourceLocation) error {
	tokens := ctx.NewVectorOrPanic(nil)
	lexer := mellifera.NewLexer(ctx, source, location)

	token, err := lexer.NextToken()
	if err != nil {
		return err
	}
	for token.Kind != mellifera.TOKEN_EOF {
		err = tokens.Push(token.IntoValue(ctx))
		if err != nil {
			return err
		}
		token, err = lexer.NextToken()
		if err != nil {
			return err
		}
	}

	var sb strings.Builder
	indent := "    "
	encoder := mellifera.NewCombEncoder(&sb)
	encoder.Indent = &indent
	encoder.Separator = " "
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
	indent := "    "
	encoder := mellifera.NewCombEncoder(&sb)
	encoder.Indent = &indent
	encoder.Separator = " "
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
	env := mellifera.NewEnvironment(ctx.BaseEnvironment)
	return program.Eval(ctx, env)
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
  -e, --env         Display the Mellifera environment and exit.
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
			// -c='println("hello 🐝");'
			if m[1] != "" {
				cmds = &m[1]
				argv = append([]string{os.Args[0]}, os.Args[argi+1:]...)
				break
			}

			// -c 'println("hello 🐝");'
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
	ctx.BaseEnvironment.Let("exit", BuiltinExit(ctx))
	ctx.BaseEnvironment.Let("import", BuiltinImport(ctx))
	ctx.BaseEnvironment.Let("input", BuiltinInput(ctx))
	ctx.BaseEnvironment.Let("inputln", BuiltinInputln(ctx))
	ctx.BaseEnvironment.Let("fs", ctx.NewMapOrPanic([]mellifera.MapPair{
		{ctx.NewString("read"), BuiltinFsRead(ctx)},
		{ctx.NewString("write"), BuiltinFsWrite(ctx)},
		{ctx.NewString("append"), BuiltinFsAppend(ctx)},
	}).Freeze())

	argvIntoValue := func() mellifera.Value {
		result := ctx.NewVectorOrPanic(nil)
		for _, arg := range argv {
			_ = result.Push(ctx.NewString(arg))
		}
		return result.Freeze()
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
	module := ctx.NewMapOrPanic([]mellifera.MapPair{
		{ctx.NewString("path"), ctx.NewString(path)},
		{ctx.NewString("file"), ctx.NewString(filepath.Base(path))},
		{ctx.NewString("directory"), ctx.NewString(filepath.Dir(path))},
	}).Freeze()
	if err = ctx.BaseEnvironment.Set("module", module); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}

	if dumpTokens && dumpAst {
		fmt.Fprintf(os.Stderr, "error: requested token dump and AST dump which are mutually exclusive\n")
		os.Exit(1)
	} else if cmds != nil || file != nil {
		if cmds != nil && dumpTokens {
			err = dumpTokensSource(ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if cmds != nil && dumpAst {
			err = dumpAstSource(ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if cmds != nil {
			_, err = evalSource(ctx, *cmds, &mellifera.SourceLocation{"<command>", 1})
		} else if file != nil && dumpTokens {
			err = dumpTokensFile(ctx, *file)
		} else if file != nil && dumpAst {
			err = dumpAstFile(ctx, *file)
		} else if file != nil {
			_, err = evalFile(ctx, *file)
		} else {
			err = fmt.Errorf("unreachable")
		}
	} else if dumpTokens {
		fmt.Fprintf(os.Stderr, "error: requested token dump without a command or file path\n")
		os.Exit(1)
	} else if dumpAst {
		fmt.Fprintf(os.Stderr, "error: requested AST dump without a command or file path\n")
		os.Exit(1)
	} else {
		usage(os.Stderr)
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
