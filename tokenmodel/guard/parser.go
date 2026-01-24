package guard

import (
	"fmt"
	"strconv"
)

// Node is the interface for all AST nodes.
type Node interface {
	node()
	String() string
}

// BinaryOp represents a binary operation (&&, ||, >=, etc.).
type BinaryOp struct {
	Op    string
	Left  Node
	Right Node
}

func (b *BinaryOp) node() {}
func (b *BinaryOp) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Op, b.Right.String())
}

// UnaryOp represents a unary operation (!).
type UnaryOp struct {
	Op      string
	Operand Node
}

func (u *UnaryOp) node() {}
func (u *UnaryOp) String() string {
	return fmt.Sprintf("(%s%s)", u.Op, u.Operand.String())
}

// IndexExpr represents array/map indexing (obj[key]).
type IndexExpr struct {
	Object Node
	Index  Node
}

func (i *IndexExpr) node() {}
func (i *IndexExpr) String() string {
	return fmt.Sprintf("%s[%s]", i.Object.String(), i.Index.String())
}

// FieldExpr represents field access (obj.field).
type FieldExpr struct {
	Object Node
	Field  string
}

func (f *FieldExpr) node() {}
func (f *FieldExpr) String() string {
	return fmt.Sprintf("%s.%s", f.Object.String(), f.Field)
}

// CallExpr represents a function call (func(args...)).
type CallExpr struct {
	Func string
	Args []Node
}

func (c *CallExpr) node() {}
func (c *CallExpr) String() string {
	args := ""
	for i, arg := range c.Args {
		if i > 0 {
			args += ", "
		}
		args += arg.String()
	}
	return fmt.Sprintf("%s(%s)", c.Func, args)
}

// Identifier represents a variable reference.
type Identifier struct {
	Name string
}

func (i *Identifier) node() {}
func (i *Identifier) String() string {
	return i.Name
}

// NumberLit represents a numeric literal.
type NumberLit struct {
	Value int64
}

func (n *NumberLit) node() {}
func (n *NumberLit) String() string {
	return strconv.FormatInt(n.Value, 10)
}

// StringLit represents a string literal.
type StringLit struct {
	Value string
}

func (s *StringLit) node() {}
func (s *StringLit) String() string {
	return fmt.Sprintf("%q", s.Value)
}

// BoolLit represents a boolean literal.
type BoolLit struct {
	Value bool
}

func (b *BoolLit) node() {}
func (b *BoolLit) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// Parser parses guard expressions into an AST.
type Parser struct {
	lexer   *Lexer
	current Token
	peek    Token
	errors  []string
	depth   int
}

const maxDepth = 100

// NewParser creates a new parser for the given input.
func NewParser(input string) *Parser {
	p := &Parser{
		lexer:  NewLexer(input),
		errors: []string{},
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) addError(msg string) {
	p.errors = append(p.errors, fmt.Sprintf("parse error at position %d: %s", p.current.Pos, msg))
}

// Errors returns any parsing errors.
func (p *Parser) Errors() []string {
	return p.errors
}

// Parse parses the input and returns the AST root.
func (p *Parser) Parse() (Node, error) {
	node := p.parseExpression()
	if len(p.errors) > 0 {
		return nil, fmt.Errorf("%s", p.errors[0])
	}
	if p.current.Type != TokenEOF {
		return nil, fmt.Errorf("unexpected token at position %d: %q", p.current.Pos, p.current.Literal)
	}
	return node, nil
}

func (p *Parser) parseExpression() Node {
	p.depth++
	if p.depth > maxDepth {
		p.addError("expression too deeply nested")
		return nil
	}
	defer func() { p.depth-- }()

	return p.parseOr()
}

func (p *Parser) parseOr() Node {
	left := p.parseAnd()
	if left == nil {
		return nil
	}

	for p.current.Type == TokenOr {
		op := p.current.Literal
		p.nextToken()
		right := p.parseAnd()
		if right == nil {
			return nil
		}
		left = &BinaryOp{Op: op, Left: left, Right: right}
	}

	return left
}

func (p *Parser) parseAnd() Node {
	left := p.parseComparison()
	if left == nil {
		return nil
	}

	for p.current.Type == TokenAnd {
		op := p.current.Literal
		p.nextToken()
		right := p.parseComparison()
		if right == nil {
			return nil
		}
		left = &BinaryOp{Op: op, Left: left, Right: right}
	}

	return left
}

func (p *Parser) parseComparison() Node {
	left := p.parseAdditive()
	if left == nil {
		return nil
	}

	if isComparisonOp(p.current.Type) {
		op := p.current.Literal
		p.nextToken()
		right := p.parseAdditive()
		if right == nil {
			return nil
		}
		return &BinaryOp{Op: op, Left: left, Right: right}
	}

	return left
}

func isComparisonOp(t TokenType) bool {
	switch t {
	case TokenGTE, TokenLTE, TokenGT, TokenLT, TokenEQ, TokenNEQ:
		return true
	}
	return false
}

func (p *Parser) parseAdditive() Node {
	left := p.parseMultiplicative()
	if left == nil {
		return nil
	}

	for p.current.Type == TokenPlus || p.current.Type == TokenMinus {
		op := p.current.Literal
		p.nextToken()
		right := p.parseMultiplicative()
		if right == nil {
			return nil
		}
		left = &BinaryOp{Op: op, Left: left, Right: right}
	}

	return left
}

func (p *Parser) parseMultiplicative() Node {
	left := p.parseUnary()
	if left == nil {
		return nil
	}

	for p.current.Type == TokenStar || p.current.Type == TokenSlash || p.current.Type == TokenPercent {
		op := p.current.Literal
		p.nextToken()
		right := p.parseUnary()
		if right == nil {
			return nil
		}
		left = &BinaryOp{Op: op, Left: left, Right: right}
	}

	return left
}

func (p *Parser) parseUnary() Node {
	if p.current.Type == TokenNot || p.current.Type == TokenMinus {
		op := p.current.Literal
		p.nextToken()
		operand := p.parseUnary()
		if operand == nil {
			return nil
		}
		return &UnaryOp{Op: op, Operand: operand}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Node {
	left := p.parsePrimary()
	if left == nil {
		return nil
	}

	for {
		switch p.current.Type {
		case TokenLBracket:
			p.nextToken()
			index := p.parseExpression()
			if index == nil {
				return nil
			}
			if p.current.Type != TokenRBracket {
				p.addError("expected ']'")
				return nil
			}
			p.nextToken()
			left = &IndexExpr{Object: left, Index: index}

		case TokenDot:
			p.nextToken()
			if p.current.Type != TokenIdentifier {
				p.addError("expected identifier after '.'")
				return nil
			}
			field := p.current.Literal
			p.nextToken()
			left = &FieldExpr{Object: left, Field: field}

		case TokenLParen:
			// Function call - left must be an identifier
			ident, ok := left.(*Identifier)
			if !ok {
				p.addError("expected function name before '('")
				return nil
			}
			p.nextToken()
			args := p.parseArguments()
			if args == nil && len(p.errors) > 0 {
				return nil
			}
			if p.current.Type != TokenRParen {
				p.addError("expected ')'")
				return nil
			}
			p.nextToken()
			left = &CallExpr{Func: ident.Name, Args: args}

		default:
			return left
		}
	}
}

func (p *Parser) parseArguments() []Node {
	args := []Node{}

	if p.current.Type == TokenRParen {
		return args
	}

	first := p.parseExpression()
	if first == nil {
		return nil
	}
	args = append(args, first)

	for p.current.Type == TokenComma {
		p.nextToken()
		arg := p.parseExpression()
		if arg == nil {
			return nil
		}
		args = append(args, arg)
	}

	return args
}

func (p *Parser) parsePrimary() Node {
	switch p.current.Type {
	case TokenIdentifier:
		name := p.current.Literal
		p.nextToken()
		return &Identifier{Name: name}

	case TokenNumber:
		val, err := strconv.ParseInt(p.current.Literal, 10, 64)
		if err != nil {
			p.addError(fmt.Sprintf("invalid number: %s", p.current.Literal))
			return nil
		}
		p.nextToken()
		return &NumberLit{Value: val}

	case TokenString:
		val := p.current.Literal
		p.nextToken()
		return &StringLit{Value: val}

	case TokenTrue:
		p.nextToken()
		return &BoolLit{Value: true}

	case TokenFalse:
		p.nextToken()
		return &BoolLit{Value: false}

	case TokenLParen:
		p.nextToken()
		expr := p.parseExpression()
		if expr == nil {
			return nil
		}
		if p.current.Type != TokenRParen {
			p.addError("expected ')'")
			return nil
		}
		p.nextToken()
		return expr

	default:
		p.addError(fmt.Sprintf("unexpected token: %q", p.current.Literal))
		return nil
	}
}
