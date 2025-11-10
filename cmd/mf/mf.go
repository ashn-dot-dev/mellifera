package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	//"ashn.dev/mellifera"
)

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
}
