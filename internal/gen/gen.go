// Package gen provides functions for generating go source code
//
// The gen package provides wrapper functions around the go/ast and
// go/token packages to reduce boilerplate.
package gen

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"reflect"
	"strconv"
	"strings"
)

// TypeDecl generates a type declaration with the given name.
func TypeDecl(name *ast.Ident, typ ast.Expr) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: name,
				Type: typ,
			},
		},
	}
}

// Struct creates a struct{} expression. The arguments are a series
// of name/type/tag tuples. Name must be of type *ast.Ident, type
// must be of type ast.Expr, and tag must be of type *ast.BasicLit,
// The number of arguments must be a multiple of 3, or a run-time
// panic will occur.
func Struct(args ...ast.Expr) *ast.StructType {
	fields := new(ast.FieldList)
	if len(args)%3 != 0 {
		panic("Number of args to FieldList must be a multiple of 3, got " + strconv.Itoa(len(args)))
	}
	for i := 0; i < len(args); i += 3 {
		var field ast.Field
		name, typ, tag := args[i], args[i+1], args[i+2]
		if name != nil {
			field.Names = []*ast.Ident{name.(*ast.Ident)}
		}
		if typ != nil {
			field.Type = typ
		}
		if tag != nil {
			field.Tag = tag.(*ast.BasicLit)
		}
		fields.List = append(fields.List, &field)
	}
	return &ast.StructType{Fields: fields}
}

// FieldList generates a field list from strings in the form "[name]
// expr".
func FieldList(fields ...string) (*ast.FieldList, error) {
	result := &ast.FieldList{List: []*ast.Field{}}
	for _, s := range fields {
		parts := strings.SplitN(s, " ", 2)
		if len(parts) == 0 {
			return nil, fmt.Errorf("empty field list item %q", s)
		}
		var names []*ast.Ident
		typeExpr, err := parser.ParseExpr(parts[len(parts)-1])
		if err != nil {
			return nil, fmt.Errorf("could not parse type in %q: %v", s, err)
		}
		if len(parts) > 1 {
			names = []*ast.Ident{ast.NewIdent(parts[0])}
		}
		result.List = append(result.List, &ast.Field{
			Names: names,
			Type:  typeExpr,
		})
	}
	return result, nil
}

// String generates a literal string. If the string contains a double
// quote, backticks are used for quoting instead.
func String(s string) *ast.BasicLit {
	if strings.Contains(s, "\"") && !strings.Contains(s, "`") {
		return &ast.BasicLit{Kind: token.STRING, Value: "`" + s + "`"}
	}
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

// Public turns a string into a public (uppercase)
// identifier.
func Public(name string) *ast.Ident {
	return ast.NewIdent(strings.Title(name))
}

func constDecl(kind token.Token, args ...string) *ast.GenDecl {
	decl := ast.GenDecl{Tok: token.CONST}

	if len(args)%3 != 0 {
		panic("Number of values passed to ConstString must be a multiple of 3")
	}
	for i := 0; i < len(args); i += 3 {
		name, typ, val := args[i], args[i+1], args[i+2]
		lit := &ast.BasicLit{Kind: kind}
		if kind == token.STRING {
			lit.Value = strconv.Quote(val)
		} else {
			lit.Value = val
		}
		a := &ast.ValueSpec{
			Names:  []*ast.Ident{ast.NewIdent(name)},
			Values: []ast.Expr{lit},
		}
		if typ != "" {
			a.Type = ast.NewIdent(typ)
		}
		decl.Specs = append(decl.Specs, a)
	}

	if len(decl.Specs) > 1 {
		decl.Lparen = 1
	}

	return &decl
}

// SimpleType creates an identifier suitable
// for use as a type expression.
func SimpleType(name string) ast.Expr {
	return ast.NewIdent(name)
}

// ConstInt creates a series of numeric const declarations from
// the name/value pairs in args.
func ConstInt(args ...string) *ast.GenDecl {
	return constDecl(token.INT, args...)
}

// ConstString creates a series of string const declarations from
// the name/value pairs in args.
func ConstString(args ...string) *ast.GenDecl {
	return constDecl(token.STRING, args...)
}

// ConstFloat creates a series of floating-point const
// declarations from the name/value pairs in args.
func ConstFloat(args ...string) *ast.GenDecl {
	return constDecl(token.FLOAT, args...)
}

// ConstChar creates a series of character const
// declarations from the name/value pairs in args.
func ConstChar(args ...string) *ast.GenDecl {
	return constDecl(token.CHAR, args...)
}

// ConstImaginary creates a series of imaginary const
// declarations from the name/value pairs in args.
func ConstImaginary(args ...string) *ast.GenDecl {
	return constDecl(token.IMAG, args...)
}

type Function struct {
	name, receiver, godoc string
	args, returns         []string
	body                  string
}

func Func(name string) *Function {
	return &Function{name: name}
}

// Decl generates Go source for a Func.  an error is returned if the
// body, or parameters cannot be parsed.
func (fn *Function) Decl() (*ast.FuncDecl, error) {
	var err error
	var comments *ast.CommentGroup

	if fn.name == "" {
		return nil, errors.New("function name unset")
	}
	if len(fn.body) == 0 {
		return nil, fmt.Errorf("function body for %s unset")
	}

	if fn.godoc != "" {
		comments = &ast.CommentGroup{List: []*ast.Comment{}}
		for _, line := range strings.Split(fn.godoc, "\n") {
			comments.List = append(comments.List, &ast.Comment{Text: line})
		}
	}
	fl := func(args ...string) (list *ast.FieldList) {
		if len(args) == 0 || len(args[0]) == 0 || err != nil {
			return nil
		}
		list, err = FieldList(args...)
		return list
	}
	args := fl(fn.args...)
	returns := fl(fn.returns...)
	receiver := fl(fn.receiver)
	if err != nil {
		return nil, err
	}
	body, err := parseBlock(fn.body)
	if err != nil {
		return nil, fmt.Errorf("could not parse function body of %s: %v", fn.name, err)
	}
	return &ast.FuncDecl{
		Doc:  comments,
		Recv: receiver,
		Name: ast.NewIdent(fn.name),
		Type: &ast.FuncType{
			Params:  args,
			Results: returns,
		},
		Body: body,
	}, nil
}

// Body sets the body of a function. The body should not include
// enclosing braces.
func (fn *Function) Body(format string, v ...interface{}) *Function {
	fn.body = fmt.Sprintf(format, v...)
	return fn
}

// Returns sets the return values of a function. Each return
// value should be a string matching the Go syntax for a
// single return value.
func (fn *Function) Returns(values ...string) *Function {
	fn.returns = values
	return fn
}

// Comments sets the Godoc comments for the function.
func (fn *Function) Comment(s string) *Function {
	fn.godoc = s
	return fn
}

// Args sets the arguments that a function takes.
func (fn *Function) Args(args ...string) *Function {
	fn.args = args
	return fn
}

// Receiver turns the function into a method operating on
// the specified type.
func (fn *Function) Receiver(receiver string) *Function {
	fn.receiver = receiver
	return fn
}

func parseBlock(s string) (*ast.BlockStmt, error) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "package tmp\nfunc _block() {\n%s\n}", s)
	file, err := parser.ParseFile(token.NewFileSet(), "", buf.Bytes(), 0)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		if decl, ok := decl.(*ast.FuncDecl); ok {
			return decl.Body, nil
		}
	}
	return nil, fmt.Errorf("parse error: no function found in %q", buf.Bytes())
}

// ExprString converts an ast.Expr to the Go source it represents.
func ExprString(expr ast.Expr) string {
	var buf bytes.Buffer
	fs := token.NewFileSet()
	printer.Fprint(&buf, fs, expr)
	return buf.String()
}

// TagKey gets the struct tag item with the
// given key.
func TagKey(field *ast.Field, key string) string {
	if field.Tag != nil {
		return ""
	}
	return reflect.StructTag(field.Tag.Value).Get(key)
}
