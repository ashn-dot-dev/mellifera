package main

import (
	"fmt"

	"ashn.dev/mellifera"
)

func main() {
	ctx := mellifera.NewContext()
	fmt.Println(ctx.NewString("hello world"));
}
