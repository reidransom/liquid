# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

This is a pure Go implementation of Shopify Liquid templates, originally developed for the Gojekyll static site generator. It provides a template engine that parses and renders Liquid templates with support for custom filters, tags, and drops.

## Development Commands

### Testing
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./parser
go test ./render
go test ./expressions

# Run tests with coverage
go test -cov ./...
```

### Linting
```bash
# Lint the entire codebase
golangci-lint run

# Lint must pass before commits
make lint
```

### Building
```bash
# Build all packages
go build -v ./...

# Build the CLI tool
go build -v ./cmd/liquid

# Install the CLI tool
go install ./cmd/liquid
```

### Code Generation
```bash
# Re-generate lexers and parser (required after editing scanner.rl or expressions.y)
go generate ./...

# Test the scanner directly (requires Ragel: brew install ragel)
cd expressions
ragel -Z scanner.rl && go test
```

### Pre-commit Workflow
```bash
# Run both lint and tests
make pre-commit
```

### Development Setup
```bash
# Install dependencies and development tools
make setup

# This installs:
# - golang.org/x/tools/cmd/stringer
# - golang.org/x/tools/cmd/goyacc
```

## Architecture

### Core Pipeline: Source → Tokens → AST → Render Tree → Output

1. **Scanning/Lexing** (`parser/` package): Tokenizes template source into tags, objects, and text
2. **Parsing** (`parser/` package): Converts tokens into an Abstract Syntax Tree (AST)
3. **Expression Parsing** (`expressions/` package): Parses expressions within tags and objects (e.g., `a.b[c]`, `site.pages | reverse`)
4. **Compilation** (`render/` package): Transforms AST into an executable render tree
5. **Rendering** (`render/` package): Executes the render tree with variable bindings to produce output

### Key Components

**Engine** (`engine.go`): Main entry point. Creates and configures the template engine with filters and tags.

**Template** (`template.go`): Compiled template that can be rendered with variable bindings.

**Parser** (`parser/` package): 
- Uses Ragel for lexical scanning (expressions/scanner.rl)
- Handles nested block structures (e.g., `{% if %}...{% elsif %}...{% endif %}`)
- Manages special blocks like `{% raw %}` and `{% comment %}`

**Expressions** (`expressions/` package):
- Parses the Liquid expression language used in `{{ }}` and tag arguments
- Implements filter chains, property access, array indexing
- Uses yacc-generated parser (expressions.y)

**Render** (`render/` package):
- Executes compiled templates
- Manages variable context and scope
- Implements auto-escaping when configured
- Uses trimWriter to handle whitespace control

**Filters** (`filters/` package): Implements standard Liquid filters (e.g., `append`, `downcase`, `split`).

**Tags** (`tags/` package): Implements standard Liquid tags like `if`, `for`, `assign`, `include`.

**Values** (`values/` package): Type system and conversions for Liquid values.

**Evaluator** (`evaluator/` package): Evaluates expressions in a variable context.

### Custom Extensions

Custom filters and tags are registered on the Engine:
- `engine.RegisterFilter(name, fn)` - Function taking 1+ inputs, returning 1-2 outputs (value, [error])
- `engine.RegisterTag(name, renderer)` - For simple tags like `{% mytag %}`
- `engine.RegisterBlock(name, renderer)` - For block tags like `{% myblock %}...{% endmyblock %}`

### Template Store Pattern

Supports pluggable template storage (filesystem, database, embedded FS, etc.):
- Implement `TemplateStore` interface with `ReadTemplate(templatename string) ([]byte, error)`
- Register with `engine.RegisterTemplateStore(store)`
- Default is `FileTemplateStore`

### Testing Strategy

- Unit tests are co-located with implementation files (e.g., `engine_test.go`, `parser/parser_test.go`)
- Many test cases are derived from official Liquid documentation
- Example tests in `engine_examples_test.go` serve as usage documentation

## Go Version

Requires Go 1.23 (see go.mod)

## Cyclomatic Complexity

The `.golangci.yml` disables `gocyclo` checks for generated functions (e.g., `expressions/scanner.go`), hand-written parsers, and generic interpreter functions where high complexity is unavoidable.
