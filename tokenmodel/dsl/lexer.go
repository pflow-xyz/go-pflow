// Package dsl implements an S-expression DSL for defining token model schemas.
package dsl

import (
	"fmt"
	"unicode"
)

// TokenType represents the type of a lexer token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenLParen   // (
	TokenRParen   // )
	TokenArrow    // ->
	TokenKeyword  // :type, :guard, :keys, etc.
	TokenSymbol   // schema, version, states, etc.
	TokenString   // "..." (legacy, still supported)
	TokenNumber   // 123, -456
	TokenGuard    // {...} guard expression
)

// Token represents a single token from the lexer.
type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%v, %q, %d}", t.Type, t.Literal, t.Pos)
}

// Lexer tokenizes S-expression DSL input.
type Lexer struct {
	input   string
	pos     int
	readPos int
	ch      byte
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	// Skip from ; to end of line
	for l.ch != 0 && l.ch != '\n' {
		l.readChar()
	}
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	for {
		l.skipWhitespace()

		// Handle comments
		if l.ch == ';' {
			l.skipComment()
			continue
		}
		break
	}

	pos := l.pos
	var tok Token

	switch l.ch {
	case 0:
		tok = Token{Type: TokenEOF, Literal: "", Pos: pos}
	case '(':
		tok = Token{Type: TokenLParen, Literal: "(", Pos: pos}
		l.readChar()
	case ')':
		tok = Token{Type: TokenRParen, Literal: ")", Pos: pos}
		l.readChar()
	case '-':
		if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TokenArrow, Literal: "->", Pos: pos}
			l.readChar()
		} else if isDigit(l.peekChar()) {
			// Negative number
			l.readChar()
			num := "-" + l.readNumber()
			tok = Token{Type: TokenNumber, Literal: num, Pos: pos}
		} else {
			// Treat as symbol
			tok = Token{Type: TokenSymbol, Literal: l.readSymbol(), Pos: pos}
		}
	case ':':
		// Keyword like :type, :guard
		l.readChar()
		keyword := ":" + l.readSymbol()
		tok = Token{Type: TokenKeyword, Literal: keyword, Pos: pos}
	case '"':
		l.readChar() // consume opening quote
		literal := l.readString('"')
		tok = Token{Type: TokenString, Literal: literal, Pos: pos}
	case '{':
		l.readChar() // consume opening brace
		literal := l.readGuard()
		tok = Token{Type: TokenGuard, Literal: literal, Pos: pos}
	default:
		if isDigit(l.ch) {
			num := l.readNumber()
			return Token{Type: TokenNumber, Literal: num, Pos: pos}
		} else if isSymbolStart(l.ch) {
			sym := l.readSymbol()
			return Token{Type: TokenSymbol, Literal: sym, Pos: pos}
		} else {
			// Unknown character, skip it
			tok = Token{Type: TokenEOF, Literal: string(l.ch), Pos: pos}
			l.readChar()
		}
	}

	return tok
}

func (l *Lexer) readSymbol() string {
	start := l.pos
	for isSymbolChar(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readString(quote byte) string {
	var result []byte
	for l.ch != 0 && l.ch != quote {
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '\\':
				result = append(result, '\\')
			case '"':
				result = append(result, '"')
			default:
				result = append(result, l.ch)
			}
		} else {
			result = append(result, l.ch)
		}
		l.readChar()
	}
	if l.ch == quote {
		l.readChar() // consume closing quote
	}
	return string(result)
}

// readGuard reads a guard expression enclosed in {...}
// Handles nested braces for complex expressions.
func (l *Lexer) readGuard() string {
	var result []byte
	depth := 1 // already consumed opening {
	for l.ch != 0 && depth > 0 {
		if l.ch == '{' {
			depth++
		} else if l.ch == '}' {
			depth--
			if depth == 0 {
				l.readChar() // consume closing brace
				break
			}
		}
		result = append(result, l.ch)
		l.readChar()
	}
	return string(result)
}

func isSymbolStart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isSymbolChar(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' || ch == '-' || ch == '[' || ch == ']' || ch == '.'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// Tokenize returns all tokens from the input.
func Tokenize(input string) []Token {
	l := NewLexer(input)
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens
}
