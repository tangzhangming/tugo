package lexer

import (
	"unicode"
)

// Lexer 词法分析器
type Lexer struct {
	input   string
	pos     int  // 当前位置
	readPos int  // 下一个读取位置
	ch      byte // 当前字符
	line    int  // 当前行号
	column  int  // 当前列号
}

// New 创建一个新的词法分析器
func New(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

// readChar 读取下一个字符
func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
	l.column++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	}
}

// peekChar 查看下一个字符但不移动位置
func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

// NextToken 获取下一个 token
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_EQ, Literal: "==", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_ASSIGN, l.ch)
		}
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_PLUS_ASSIGN, Literal: "+=", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '+' {
			l.readChar()
			tok = Token{Type: TOKEN_INC, Literal: "++", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_PLUS, l.ch)
		}
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MINUS_ASSIGN, Literal: "-=", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '-' {
			l.readChar()
			tok = Token{Type: TOKEN_DEC, Literal: "--", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_ARROW, Literal: "->", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_MINUS, l.ch)
		}
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_ASTERISK_ASSIGN, Literal: "*=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_ASTERISK, l.ch)
		}
	case '/':
		if l.peekChar() == '/' {
			tok.Type = TOKEN_COMMENT
			tok.Literal = l.readLineComment()
			tok.Line = l.line
			tok.Column = l.column
			return tok
		} else if l.peekChar() == '*' {
			tok.Type = TOKEN_COMMENT
			tok.Literal = l.readBlockComment()
			tok.Line = l.line
			tok.Column = l.column
			return tok
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_SLASH_ASSIGN, Literal: "/=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_SLASH, l.ch)
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_PERCENT_ASSIGN, Literal: "%=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_PERCENT, l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_NOT_EQ, Literal: "!=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_NOT, l.ch)
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_LT_EQ, Literal: "<=", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '<' {
			l.readChar()
			tok = Token{Type: TOKEN_SHL, Literal: "<<", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '-' {
			l.readChar()
			tok = Token{Type: TOKEN_ARROW, Literal: "<-", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_LT, l.ch)
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_GT_EQ, Literal: ">=", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_SHR, Literal: ">>", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_GT, l.ch)
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			tok = Token{Type: TOKEN_AND, Literal: "&&", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_BIT_AND, l.ch)
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok = Token{Type: TOKEN_OR, Literal: "||", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_BIT_OR, l.ch)
		}
	case '^':
		tok = l.newToken(TOKEN_BIT_XOR, l.ch)
	case '~':
		tok = l.newToken(TOKEN_BIT_NOT, l.ch)
	case ',':
		tok = l.newToken(TOKEN_COMMA, l.ch)
	case ';':
		tok = l.newToken(TOKEN_SEMICOLON, l.ch)
	case ':':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_DEFINE, Literal: ":=", Line: tok.Line, Column: tok.Column}
		} else if l.peekChar() == ':' {
			l.readChar()
			tok = Token{Type: TOKEN_DOUBLE_COLON, Literal: "::", Line: tok.Line, Column: tok.Column}
		} else {
			tok = l.newToken(TOKEN_COLON, l.ch)
		}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			if l.peekChar() == '.' {
				l.readChar()
				tok = Token{Type: TOKEN_ELLIPSIS, Literal: "...", Line: tok.Line, Column: tok.Column}
			} else {
				tok = Token{Type: TOKEN_ILLEGAL, Literal: "..", Line: tok.Line, Column: tok.Column}
			}
		} else {
			tok = l.newToken(TOKEN_DOT, l.ch)
		}
	case '(':
		tok = l.newToken(TOKEN_LPAREN, l.ch)
	case ')':
		tok = l.newToken(TOKEN_RPAREN, l.ch)
	case '[':
		tok = l.newToken(TOKEN_LBRACKET, l.ch)
	case ']':
		tok = l.newToken(TOKEN_RBRACKET, l.ch)
	case '{':
		tok = l.newToken(TOKEN_LBRACE, l.ch)
	case '}':
		tok = l.newToken(TOKEN_RBRACE, l.ch)
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
	case '\'':
		tok.Type = TOKEN_CHAR
		tok.Literal = l.readChar2()
	case '`':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readRawString()
	case 0:
		tok.Literal = ""
		tok.Type = TOKEN_EOF
	default:
		if l.isLetter(l.ch) || l.ch == '$' {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupIdent(tok.Literal)
			return tok
		} else if l.isDigit(l.ch) {
			tok.Literal, tok.Type = l.readNumber()
			return tok
		} else {
			tok = l.newToken(TOKEN_ILLEGAL, l.ch)
		}
	}

	l.readChar()
	return tok
}

// newToken 创建新的 token
func (l *Lexer) newToken(tokenType TokenType, ch byte) Token {
	return Token{Type: tokenType, Literal: string(ch), Line: l.line, Column: l.column}
}

// skipWhitespace 跳过空白字符
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// readIdentifier 读取标识符
func (l *Lexer) readIdentifier() string {
	pos := l.pos
	// 允许 $ 开头
	if l.ch == '$' {
		l.readChar()
	}
	for l.isLetter(l.ch) || l.isDigit(l.ch) || l.ch == '_' || l.ch == '$' {
		l.readChar()
	}
	return l.input[pos:l.pos]
}

// readNumber 读取数字（整数或浮点数）
func (l *Lexer) readNumber() (string, TokenType) {
	pos := l.pos
	tokenType := TOKEN_INT

	// 处理十六进制、八进制、二进制
	if l.ch == '0' {
		l.readChar()
		if l.ch == 'x' || l.ch == 'X' {
			l.readChar()
			for l.isHexDigit(l.ch) {
				l.readChar()
			}
			return l.input[pos:l.pos], TOKEN_INT
		} else if l.ch == 'b' || l.ch == 'B' {
			l.readChar()
			for l.ch == '0' || l.ch == '1' {
				l.readChar()
			}
			return l.input[pos:l.pos], TOKEN_INT
		} else if l.ch == 'o' || l.ch == 'O' {
			l.readChar()
			for l.ch >= '0' && l.ch <= '7' {
				l.readChar()
			}
			return l.input[pos:l.pos], TOKEN_INT
		}
	}

	for l.isDigit(l.ch) {
		l.readChar()
	}

	// 浮点数
	if l.ch == '.' && l.isDigit(l.peekChar()) {
		tokenType = TOKEN_FLOAT
		l.readChar()
		for l.isDigit(l.ch) {
			l.readChar()
		}
	}

	// 科学计数法
	if l.ch == 'e' || l.ch == 'E' {
		tokenType = TOKEN_FLOAT
		l.readChar()
		if l.ch == '+' || l.ch == '-' {
			l.readChar()
		}
		for l.isDigit(l.ch) {
			l.readChar()
		}
	}

	return l.input[pos:l.pos], tokenType
}

// readString 读取双引号字符串
func (l *Lexer) readString() string {
	pos := l.pos
	l.readChar() // 跳过开头的 "
	for {
		if l.ch == '"' {
			break
		}
		if l.ch == '\\' {
			l.readChar() // 跳过转义字符
		}
		if l.ch == 0 {
			break
		}
		l.readChar()
	}
	result := l.input[pos : l.pos+1]
	return result
}

// readChar2 读取单引号字符
func (l *Lexer) readChar2() string {
	pos := l.pos
	l.readChar() // 跳过开头的 '
	for {
		if l.ch == '\'' {
			break
		}
		if l.ch == '\\' {
			l.readChar() // 跳过转义字符
		}
		if l.ch == 0 {
			break
		}
		l.readChar()
	}
	result := l.input[pos : l.pos+1]
	return result
}

// readRawString 读取反引号原始字符串
func (l *Lexer) readRawString() string {
	pos := l.pos
	l.readChar() // 跳过开头的 `
	for l.ch != '`' && l.ch != 0 {
		l.readChar()
	}
	result := l.input[pos : l.pos+1]
	return result
}

// readLineComment 读取单行注释
func (l *Lexer) readLineComment() string {
	pos := l.pos
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	return l.input[pos:l.pos]
}

// readBlockComment 读取块注释
func (l *Lexer) readBlockComment() string {
	pos := l.pos
	l.readChar() // 跳过 /
	l.readChar() // 跳过 *
	for {
		if l.ch == '*' && l.peekChar() == '/' {
			l.readChar()
			l.readChar()
			break
		}
		if l.ch == 0 {
			break
		}
		l.readChar()
	}
	return l.input[pos:l.pos]
}

// isLetter 判断是否为字母
func (l *Lexer) isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

// isDigit 判断是否为数字
func (l *Lexer) isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isHexDigit 判断是否为十六进制数字
func (l *Lexer) isHexDigit(ch byte) bool {
	return l.isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// Tokenize 将输入字符串转换为 token 列表
func Tokenize(input string) []Token {
	l := New(input)
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TOKEN_EOF {
			break
		}
	}
	return tokens
}
