// Package guard implements a secure guard expression evaluator for Petri net transitions.
package guard

import (
	"fmt"
	"unicode"
)

// TokenType represents the type of a lexer token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdentifier
	TokenNumber
	TokenString    // "..." or '...'
	TokenLParen    // (
	TokenRParen    // )
	TokenLBracket  // [
	TokenRBracket  // ]
	TokenDot       // .
	TokenComma     // ,
	TokenGTE       // >=
	TokenLTE       // <=
	TokenGT        // >
	TokenLT        // <
	TokenEQ        // ==
	TokenNEQ       // !=
	TokenAnd       // &&
	TokenOr        // ||
	TokenNot       // !
	TokenTrue      // true
	TokenFalse     // false
	TokenPlus      // +
	TokenMinus     // -
	TokenStar      // *
	TokenSlash     // /
	TokenPercent   // %
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

// Lexer tokenizes guard expressions.
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

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

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
	case '[':
		tok = Token{Type: TokenLBracket, Literal: "[", Pos: pos}
		l.readChar()
	case ']':
		tok = Token{Type: TokenRBracket, Literal: "]", Pos: pos}
		l.readChar()
	case '.':
		tok = Token{Type: TokenDot, Literal: ".", Pos: pos}
		l.readChar()
	case ',':
		tok = Token{Type: TokenComma, Literal: ",", Pos: pos}
		l.readChar()
	case '+':
		tok = Token{Type: TokenPlus, Literal: "+", Pos: pos}
		l.readChar()
	case '-':
		tok = Token{Type: TokenMinus, Literal: "-", Pos: pos}
		l.readChar()
	case '*':
		tok = Token{Type: TokenStar, Literal: "*", Pos: pos}
		l.readChar()
	case '/':
		tok = Token{Type: TokenSlash, Literal: "/", Pos: pos}
		l.readChar()
	case '%':
		tok = Token{Type: TokenPercent, Literal: "%", Pos: pos}
		l.readChar()
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenGTE, Literal: ">=", Pos: pos}
		} else {
			tok = Token{Type: TokenGT, Literal: ">", Pos: pos}
		}
		l.readChar()
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenLTE, Literal: "<=", Pos: pos}
		} else {
			tok = Token{Type: TokenLT, Literal: "<", Pos: pos}
		}
		l.readChar()
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenEQ, Literal: "==", Pos: pos}
			l.readChar()
		} else {
			tok = Token{Type: TokenEOF, Literal: string(l.ch), Pos: pos}
			l.readChar()
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TokenNEQ, Literal: "!=", Pos: pos}
			l.readChar()
		} else {
			tok = Token{Type: TokenNot, Literal: "!", Pos: pos}
			l.readChar()
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			tok = Token{Type: TokenAnd, Literal: "&&", Pos: pos}
			l.readChar()
		} else {
			tok = Token{Type: TokenEOF, Literal: string(l.ch), Pos: pos}
			l.readChar()
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TokenOr, Literal: "||", Pos: pos}
			l.readChar()
		} else {
			tok = Token{Type: TokenEOF, Literal: string(l.ch), Pos: pos}
			l.readChar()
		}
	case '"', '\'':
		quote := l.ch
		l.readChar() // consume opening quote
		literal := l.readString(quote)
		tok = Token{Type: TokenString, Literal: literal, Pos: pos}
	default:
		if isLetter(l.ch) {
			literal := l.readIdentifier()
			tokType := lookupKeyword(literal)
			return Token{Type: tokType, Literal: literal, Pos: pos}
		} else if isDigit(l.ch) {
			literal := l.readNumber()
			return Token{Type: TokenNumber, Literal: literal, Pos: pos}
		} else {
			tok = Token{Type: TokenEOF, Literal: string(l.ch), Pos: pos}
			l.readChar()
		}
	}

	return tok
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) {
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
		if l.ch == '\\' && l.peekChar() == quote {
			// Handle escaped quote
			l.readChar()
		}
		result = append(result, l.ch)
		l.readChar()
	}
	if l.ch == quote {
		l.readChar() // consume closing quote
	}
	return string(result)
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func lookupKeyword(ident string) TokenType {
	switch ident {
	case "true":
		return TokenTrue
	case "false":
		return TokenFalse
	default:
		return TokenIdentifier
	}
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
