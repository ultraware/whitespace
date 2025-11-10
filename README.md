# Whitespace linter

Whitespace is a linter that checks for unnecessary newlines at the start and end of functions, if, for, etc.

## Installation guide

To install as a standalone linter, run `go install github.com/ultraware/whitespace/cmd/whitespace@latest`.

Whitespace is also included in [golangci-lint](https://github.com/golangci/golangci-lint/). Install it and enable whitespace.

## Usage

```

Example: ./whitespace ./...

Usage: whitespace [-flag] [package]

Configuration flags:
  -ignore-leading
        Do not check leading newlines
  -ignore-trailing
        Do not check trailing newlines
  -multi-func
        Check that multi line functions have a leading newline
  -multi-if
        Check that multi line if-statements have a leading newline
```
