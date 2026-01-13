package lexer

// TokenType 表示 token 的类型
type TokenType int

const (
	// 特殊 token
	TOKEN_ILLEGAL TokenType = iota
	TOKEN_EOF
	TOKEN_COMMENT

	// 标识符和字面量
	TOKEN_IDENT  // 标识符
	TOKEN_INT    // 整数
	TOKEN_FLOAT  // 浮点数
	TOKEN_STRING // 字符串
	TOKEN_CHAR   // 字符

	// 运算符
	TOKEN_ASSIGN   // =
	TOKEN_PLUS     // +
	TOKEN_MINUS    // -
	TOKEN_ASTERISK // *
	TOKEN_SLASH    // /
	TOKEN_PERCENT  // %

	TOKEN_EQ     // ==
	TOKEN_NOT_EQ // !=
	TOKEN_LT     // <
	TOKEN_GT     // >
	TOKEN_LT_EQ  // <=
	TOKEN_GT_EQ  // >=

	TOKEN_AND // &&
	TOKEN_OR  // ||
	TOKEN_NOT // !

	TOKEN_BIT_AND // &
	TOKEN_BIT_OR  // |
	TOKEN_BIT_XOR // ^
	TOKEN_BIT_NOT // ~
	TOKEN_SHL     // <<
	TOKEN_SHR     // >>

	TOKEN_PLUS_ASSIGN     // +=
	TOKEN_MINUS_ASSIGN    // -=
	TOKEN_ASTERISK_ASSIGN // *=
	TOKEN_SLASH_ASSIGN    // /=
	TOKEN_PERCENT_ASSIGN  // %=

	TOKEN_INC // ++
	TOKEN_DEC // --

	TOKEN_DEFINE       // :=
	TOKEN_ARROW        // ->
	TOKEN_FAT_ARROW    // =>
	TOKEN_DOUBLE_COLON // ::
	TOKEN_QUESTION     // ?

	// 分隔符
	TOKEN_COMMA     // ,
	TOKEN_SEMICOLON // ;
	TOKEN_COLON     // :
	TOKEN_DOT       // .
	TOKEN_ELLIPSIS  // ...

	TOKEN_LPAREN   // (
	TOKEN_RPAREN   // )
	TOKEN_LBRACKET // [
	TOKEN_RBRACKET // ]
	TOKEN_LBRACE   // {
	TOKEN_RBRACE   // }

	// 关键字
	TOKEN_FUNC     // func
	TOKEN_PUBLIC   // public
	TOKEN_STRUCT   // struct
	TOKEN_VAR      // var
	TOKEN_CONST    // const
	TOKEN_TYPE     // type
	TOKEN_PACKAGE  // package
	TOKEN_IMPORT   // import
	TOKEN_RETURN   // return
	TOKEN_IF       // if
	TOKEN_ELSE     // else
	TOKEN_FOR      // for
	TOKEN_RANGE    // range
	TOKEN_BREAK    // break
	TOKEN_CONTINUE // continue
	TOKEN_SWITCH   // switch
	TOKEN_CASE     // case
	TOKEN_DEFAULT  // default
	TOKEN_MAP      // map
	TOKEN_CHAN     // chan
	TOKEN_GO       // go
	TOKEN_DEFER    // defer
	TOKEN_SELECT   // select
	TOKEN_NIL      // nil
	TOKEN_TRUE     // true
	TOKEN_FALSE    // false
	TOKEN_MAKE     // make
	TOKEN_NEW      // new
	TOKEN_LEN      // len
	TOKEN_CAP      // cap
	TOKEN_APPEND   // append
	TOKEN_COPY     // copy
	TOKEN_DELETE   // delete
	TOKEN_INTERFACE   // interface
	TOKEN_FALLTHROUGH // fallthrough

	// class 相关关键字
	TOKEN_CLASS      // class
	TOKEN_STATIC     // static
	TOKEN_THIS       // this
	TOKEN_PRIVATE    // private
	TOKEN_PROTECTED  // protected
	TOKEN_IMPLEMENTS // implements
	TOKEN_ABSTRACT   // abstract
	TOKEN_EXTENDS    // extends
	TOKEN_SELF       // self
	TOKEN_FROM       // from (用于 import "fmt" from golang)

	// 错误处理关键字
	TOKEN_TRY   // try
	TOKEN_CATCH // catch
	TOKEN_THROW // throw

	// 模式匹配
	TOKEN_MATCH // match
)

// Token 表示一个词法单元
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

var keywords = map[string]TokenType{
	"func":      TOKEN_FUNC,
	"public":    TOKEN_PUBLIC,
	"struct":    TOKEN_STRUCT,
	"var":       TOKEN_VAR,
	"const":     TOKEN_CONST,
	"type":      TOKEN_TYPE,
	"package":   TOKEN_PACKAGE,
	"import":    TOKEN_IMPORT,
	"return":    TOKEN_RETURN,
	"if":        TOKEN_IF,
	"else":      TOKEN_ELSE,
	"for":       TOKEN_FOR,
	"range":     TOKEN_RANGE,
	"break":     TOKEN_BREAK,
	"continue":  TOKEN_CONTINUE,
	"switch":    TOKEN_SWITCH,
	"case":      TOKEN_CASE,
	"default":   TOKEN_DEFAULT,
	"map":       TOKEN_MAP,
	"chan":      TOKEN_CHAN,
	"go":        TOKEN_GO,
	"defer":     TOKEN_DEFER,
	"select":    TOKEN_SELECT,
	"nil":       TOKEN_NIL,
	"true":      TOKEN_TRUE,
	"false":     TOKEN_FALSE,
	"make":      TOKEN_MAKE,
	"new":       TOKEN_NEW,
	"len":       TOKEN_LEN,
	"cap":       TOKEN_CAP,
	"append":    TOKEN_APPEND,
	"copy":      TOKEN_COPY,
	"delete":    TOKEN_DELETE,
	"interface":   TOKEN_INTERFACE,
	"fallthrough": TOKEN_FALLTHROUGH,
	"class":      TOKEN_CLASS,
	"static":     TOKEN_STATIC,
	"this":       TOKEN_THIS,
	"private":    TOKEN_PRIVATE,
	"protected":  TOKEN_PROTECTED,
	"implements": TOKEN_IMPLEMENTS,
	"abstract":   TOKEN_ABSTRACT,
	"extends":    TOKEN_EXTENDS,
	"self":       TOKEN_SELF,
	"from":       TOKEN_FROM,
	"try":        TOKEN_TRY,
	"catch":      TOKEN_CATCH,
	"throw":      TOKEN_THROW,
	"match":      TOKEN_MATCH,
}

// LookupIdent 查找标识符是否为关键字
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

// TokenTypeName 返回 token 类型的名称
func TokenTypeName(t TokenType) string {
	names := map[TokenType]string{
		TOKEN_ILLEGAL:   "ILLEGAL",
		TOKEN_EOF:       "EOF",
		TOKEN_COMMENT:   "COMMENT",
		TOKEN_IDENT:     "IDENT",
		TOKEN_INT:       "INT",
		TOKEN_FLOAT:     "FLOAT",
		TOKEN_STRING:    "STRING",
		TOKEN_CHAR:      "CHAR",
		TOKEN_ASSIGN:    "=",
		TOKEN_PLUS:      "+",
		TOKEN_MINUS:     "-",
		TOKEN_ASTERISK:  "*",
		TOKEN_SLASH:     "/",
		TOKEN_PERCENT:   "%",
		TOKEN_EQ:        "==",
		TOKEN_NOT_EQ:    "!=",
		TOKEN_LT:        "<",
		TOKEN_GT:        ">",
		TOKEN_LT_EQ:     "<=",
		TOKEN_GT_EQ:     ">=",
		TOKEN_AND:       "&&",
		TOKEN_OR:        "||",
		TOKEN_NOT:       "!",
		TOKEN_BIT_AND:   "&",
		TOKEN_BIT_OR:    "|",
		TOKEN_BIT_XOR:   "^",
		TOKEN_BIT_NOT:   "~",
		TOKEN_SHL:       "<<",
		TOKEN_SHR:       ">>",
		TOKEN_DEFINE:       ":=",
		TOKEN_ARROW:        "->",
		TOKEN_FAT_ARROW:    "=>",
		TOKEN_DOUBLE_COLON: "::",
		TOKEN_QUESTION:     "?",
		TOKEN_COMMA:     ",",
		TOKEN_SEMICOLON: ";",
		TOKEN_COLON:     ":",
		TOKEN_DOT:       ".",
		TOKEN_ELLIPSIS:  "...",
		TOKEN_LPAREN:    "(",
		TOKEN_RPAREN:    ")",
		TOKEN_LBRACKET:  "[",
		TOKEN_RBRACKET:  "]",
		TOKEN_LBRACE:    "{",
		TOKEN_RBRACE:    "}",
		TOKEN_FUNC:      "func",
		TOKEN_PUBLIC:    "public",
		TOKEN_STRUCT:    "struct",
		TOKEN_VAR:       "var",
		TOKEN_CONST:     "const",
		TOKEN_TYPE:      "type",
		TOKEN_PACKAGE:   "package",
		TOKEN_IMPORT:    "import",
		TOKEN_RETURN:    "return",
		TOKEN_IF:        "if",
		TOKEN_ELSE:      "else",
		TOKEN_FOR:       "for",
		TOKEN_RANGE:     "range",
		TOKEN_BREAK:     "break",
		TOKEN_CONTINUE:  "continue",
		TOKEN_SWITCH:    "switch",
		TOKEN_CASE:      "case",
		TOKEN_DEFAULT:   "default",
		TOKEN_MAP:       "map",
		TOKEN_CHAN:      "chan",
		TOKEN_GO:        "go",
		TOKEN_DEFER:     "defer",
		TOKEN_SELECT:    "select",
		TOKEN_NIL:       "nil",
		TOKEN_TRUE:      "true",
		TOKEN_FALSE:     "false",
		TOKEN_CLASS:      "class",
		TOKEN_STATIC:     "static",
		TOKEN_THIS:       "this",
		TOKEN_PRIVATE:    "private",
		TOKEN_PROTECTED:  "protected",
		TOKEN_IMPLEMENTS: "implements",
		TOKEN_ABSTRACT:   "abstract",
		TOKEN_EXTENDS:    "extends",
		TOKEN_SELF:       "self",
		TOKEN_FROM:       "from",
		TOKEN_TRY:        "try",
		TOKEN_CATCH:      "catch",
		TOKEN_THROW:      "throw",
		TOKEN_MATCH:      "match",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return "UNKNOWN"
}
