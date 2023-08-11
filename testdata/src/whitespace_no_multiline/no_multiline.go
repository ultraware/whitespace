package whitespace

import "fmt"

// MultiFunc (FuncDecl), MultiIf and MultiFunc(FuncLit) want to remove newlinne.
func fn1(
	arg1 int,
	arg2 int,
) { // want "unnecessary leading newline"

	fmt.Println("Hello, World")

	if true &&
		false { // want "unnecessary leading newline"

		fmt.Println("Hello, World")
	}

	_ = func(
		arg1 int,
		arg2 int,
	) { // want "unnecessary leading newline"

		fmt.Println("Hello, World")
	}

	_ = func(
		arg1 int,
		arg2 int,
	) {
		_ = func(
			arg1 int,
			arg2 int,
		) { // want "unnecessary leading newline"

			fmt.Println("Hello, World")
		}
	}
}

// MultiFunc (FuncDecl) with comment.
func fn2(
	arg1 int,
	arg2 int,
) { // want "unnecessary leading newline"

	// A comment.
	fmt.Println("Hello, World")
}

