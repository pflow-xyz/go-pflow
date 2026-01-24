package dsl

import (
	"fmt"
	"strconv"
)

// Parser parses S-expression DSL into an AST.
type Parser struct {
	lexer *Lexer
	cur   Token
	peek  Token
}

// NewParser creates a new parser for the given input.
func NewParser(input string) *Parser {
	p := &Parser{lexer: NewLexer(input)}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.cur = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) expect(t TokenType) error {
	if p.cur.Type != t {
		return fmt.Errorf("expected %v, got %v at position %d", t, p.cur.Type, p.cur.Pos)
	}
	return nil
}

func (p *Parser) expectSymbol(s string) error {
	if p.cur.Type != TokenSymbol || p.cur.Literal != s {
		return fmt.Errorf("expected symbol %q, got %v %q at position %d", s, p.cur.Type, p.cur.Literal, p.cur.Pos)
	}
	return nil
}

// expectIdentifier accepts TokenSymbol only
func (p *Parser) expectIdentifier() (string, error) {
	if p.cur.Type == TokenSymbol {
		lit := p.cur.Literal
		p.nextToken()
		return lit, nil
	}
	return "", fmt.Errorf("expected identifier, got %v at position %d", p.cur.Type, p.cur.Pos)
}

// expectGuardExpr accepts TokenGuard only
func (p *Parser) expectGuardExpr() (string, error) {
	if p.cur.Type == TokenGuard {
		lit := p.cur.Literal
		p.nextToken()
		return lit, nil
	}
	return "", fmt.Errorf("expected guard expression {...}, got %v at position %d", p.cur.Type, p.cur.Pos)
}

// Parse parses the input and returns a SchemaNode.
func Parse(input string) (*SchemaNode, error) {
	p := NewParser(input)
	return p.parseSchema()
}

func (p *Parser) parseSchema() (*SchemaNode, error) {
	// Expect (schema "name" ...)
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	p.nextToken()

	if err := p.expectSymbol("schema"); err != nil {
		return nil, err
	}
	p.nextToken()

	name, err := p.expectIdentifier()
	if err != nil {
		return nil, fmt.Errorf("expected schema name: %w", err)
	}
	node := &SchemaNode{Name: name}

	// Parse clauses until closing paren
	for p.cur.Type != TokenRParen && p.cur.Type != TokenEOF {
		if p.cur.Type != TokenLParen {
			return nil, fmt.Errorf("expected clause starting with '(', got %v at position %d", p.cur.Type, p.cur.Pos)
		}
		p.nextToken()

		if p.cur.Type != TokenSymbol {
			return nil, fmt.Errorf("expected clause type symbol, got %v at position %d", p.cur.Type, p.cur.Pos)
		}

		switch p.cur.Literal {
		case "version":
			p.nextToken()
			version, err := p.expectIdentifier()
			if err != nil {
				return nil, fmt.Errorf("version: %w", err)
			}
			node.Version = version
			if err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			p.nextToken()

		case "states":
			p.nextToken()
			states, err := p.parseStates()
			if err != nil {
				return nil, err
			}
			node.States = states
			if err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			p.nextToken()

		case "actions":
			p.nextToken()
			actions, err := p.parseActions()
			if err != nil {
				return nil, err
			}
			node.Actions = actions
			if err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			p.nextToken()

		case "arcs":
			p.nextToken()
			arcs, err := p.parseArcs()
			if err != nil {
				return nil, err
			}
			node.Arcs = arcs
			if err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			p.nextToken()

		case "constraints":
			p.nextToken()
			constraints, err := p.parseConstraints()
			if err != nil {
				return nil, err
			}
			node.Constraints = constraints
			if err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			p.nextToken()

		default:
			return nil, fmt.Errorf("unknown clause type %q at position %d", p.cur.Literal, p.cur.Pos)
		}
	}

	return node, nil
}

func (p *Parser) parseStates() ([]*StateNode, error) {
	var states []*StateNode

	for p.cur.Type == TokenLParen {
		p.nextToken()
		if err := p.expectSymbol("state"); err != nil {
			return nil, err
		}
		p.nextToken()

		id, err := p.expectIdentifier()
		if err != nil {
			return nil, fmt.Errorf("state expects ID: %w", err)
		}
		state := &StateNode{ID: id, Kind: "data"}

		// Parse keyword-value pairs
		for p.cur.Type == TokenKeyword {
			keyword := p.cur.Literal
			p.nextToken()

			switch keyword {
			case ":type":
				typeStr, err := p.expectIdentifier()
				if err != nil {
					return nil, fmt.Errorf(":type: %w", err)
				}
				state.Type = typeStr

			case ":kind":
				if err := p.expect(TokenSymbol); err != nil {
					return nil, fmt.Errorf(":kind expects symbol (token or data): %w", err)
				}
				state.Kind = p.cur.Literal
				p.nextToken()

			case ":initial":
				val, err := p.parseValue()
				if err != nil {
					return nil, fmt.Errorf(":initial value: %w", err)
				}
				state.Initial = val

			case ":exported":
				state.Exported = true
				// No value to consume

			default:
				return nil, fmt.Errorf("unknown state keyword %q at position %d", keyword, p.cur.Pos)
			}
		}

		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		p.nextToken()

		states = append(states, state)
	}

	return states, nil
}

func (p *Parser) parseActions() ([]*ActionNode, error) {
	var actions []*ActionNode

	for p.cur.Type == TokenLParen {
		p.nextToken()
		if err := p.expectSymbol("action"); err != nil {
			return nil, err
		}
		p.nextToken()

		id, err := p.expectIdentifier()
		if err != nil {
			return nil, fmt.Errorf("action expects ID: %w", err)
		}
		action := &ActionNode{ID: id}

		// Parse keyword-value pairs
		for p.cur.Type == TokenKeyword {
			keyword := p.cur.Literal
			p.nextToken()

			switch keyword {
			case ":guard":
				guard, err := p.expectGuardExpr()
				if err != nil {
					return nil, fmt.Errorf(":guard: %w", err)
				}
				action.Guard = guard

			default:
				return nil, fmt.Errorf("unknown action keyword %q at position %d", keyword, p.cur.Pos)
			}
		}

		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		p.nextToken()

		actions = append(actions, action)
	}

	return actions, nil
}

func (p *Parser) parseArcs() ([]*ArcNode, error) {
	var arcs []*ArcNode

	for p.cur.Type == TokenLParen {
		p.nextToken()
		if err := p.expectSymbol("arc"); err != nil {
			return nil, err
		}
		p.nextToken()

		// Source
		source, err := p.expectIdentifier()
		if err != nil {
			return nil, fmt.Errorf("arc expects source: %w", err)
		}
		arc := &ArcNode{Source: source}

		// Arrow
		if err := p.expect(TokenArrow); err != nil {
			return nil, fmt.Errorf("arc expects '->': %w", err)
		}
		p.nextToken()

		// Target
		target, err := p.expectIdentifier()
		if err != nil {
			return nil, fmt.Errorf("arc expects target: %w", err)
		}
		arc.Target = target

		// Parse keyword-value pairs
		for p.cur.Type == TokenKeyword {
			keyword := p.cur.Literal
			p.nextToken()

			switch keyword {
			case ":keys":
				keys, err := p.parseIdentifierList()
				if err != nil {
					return nil, fmt.Errorf(":keys: %w", err)
				}
				arc.Keys = keys

			case ":value":
				val, err := p.expectIdentifier()
				if err != nil {
					return nil, fmt.Errorf(":value: %w", err)
				}
				arc.Value = val

			default:
				return nil, fmt.Errorf("unknown arc keyword %q at position %d", keyword, p.cur.Pos)
			}
		}

		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		p.nextToken()

		arcs = append(arcs, arc)
	}

	return arcs, nil
}

func (p *Parser) parseConstraints() ([]*ConstraintNode, error) {
	var constraints []*ConstraintNode

	for p.cur.Type == TokenLParen {
		p.nextToken()
		if err := p.expectSymbol("constraint"); err != nil {
			return nil, err
		}
		p.nextToken()

		// ID
		id, err := p.expectIdentifier()
		if err != nil {
			return nil, fmt.Errorf("constraint expects ID: %w", err)
		}
		constraint := &ConstraintNode{ID: id}

		// Expression
		expr, err := p.expectGuardExpr()
		if err != nil {
			return nil, fmt.Errorf("constraint expects expression: %w", err)
		}
		constraint.Expr = expr

		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		p.nextToken()

		constraints = append(constraints, constraint)
	}

	return constraints, nil
}

func (p *Parser) parseIdentifierList() ([]string, error) {
	// Expect (identifier identifier ...)
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	p.nextToken()

	var ids []string
	for p.cur.Type == TokenSymbol {
		ids = append(ids, p.cur.Literal)
		p.nextToken()
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	p.nextToken()

	return ids, nil
}

func (p *Parser) parseValue() (any, error) {
	switch p.cur.Type {
	case TokenNumber:
		n, err := strconv.ParseInt(p.cur.Literal, 10, 64)
		if err != nil {
			return nil, err
		}
		p.nextToken()
		return n, nil

	case TokenString:
		s := p.cur.Literal
		p.nextToken()
		return s, nil

	case TokenSymbol:
		if p.cur.Literal == "nil" {
			p.nextToken()
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected symbol %q in value position", p.cur.Literal)

	case TokenLParen:
		// Could be (map) or (list) or nested structure
		p.nextToken()
		if p.cur.Type == TokenSymbol {
			switch p.cur.Literal {
			case "map":
				p.nextToken()
				if err := p.expect(TokenRParen); err != nil {
					return nil, err
				}
				p.nextToken()
				return map[string]any{}, nil
			case "list":
				p.nextToken()
				if err := p.expect(TokenRParen); err != nil {
					return nil, err
				}
				p.nextToken()
				return []any{}, nil
			}
		}
		return nil, fmt.Errorf("unexpected list in value position at %d", p.cur.Pos)

	default:
		return nil, fmt.Errorf("unexpected token %v in value position at %d", p.cur.Type, p.cur.Pos)
	}
}
