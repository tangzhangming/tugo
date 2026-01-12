package parser

import (
	"github.com/tangzhangming/tugo/internal/i18n"
	"github.com/tangzhangming/tugo/internal/lexer"
)

// Parser 语法分析器
type Parser struct {
	l         *lexer.Lexer
	curToken  lexer.Token
	peekToken lexer.Token
	errors    []string
}

// New 创建一个新的语法分析器
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}
	// 读取两个 token，初始化 curToken 和 peekToken
	p.nextToken()
	p.nextToken()
	return p
}

// Errors 返回解析过程中的错误
func (p *Parser) Errors() []string {
	return p.errors
}

// nextToken 前进到下一个 token
func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
	// 跳过注释
	for p.peekToken.Type == lexer.TOKEN_COMMENT {
		p.peekToken = p.l.NextToken()
	}
}

// curTokenIs 检查当前 token 类型
func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

// peekTokenIs 检查下一个 token 类型
func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

// expectPeek 期望下一个 token 类型并前进
func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

// peekError 记录期望错误
func (p *Parser) peekError(t lexer.TokenType) {
	msg := i18n.T(i18n.ErrExpectedToken,
		p.peekToken.Line, p.peekToken.Column,
		lexer.TokenTypeName(t), lexer.TokenTypeName(p.peekToken.Type))
	p.errors = append(p.errors, msg)
}

// addError 添加错误
func (p *Parser) addError(msg string) {
	p.errors = append(p.errors, i18n.T(i18n.ErrGeneric,
		p.curToken.Line, p.curToken.Column, msg))
}

// ParseFile 解析整个文件
func (p *Parser) ParseFile() *File {
	file := &File{}

	// 解析 package 声明
	if p.curTokenIs(lexer.TOKEN_PACKAGE) {
		p.nextToken()
		if p.curTokenIs(lexer.TOKEN_IDENT) {
			file.Package = p.curToken.Literal
			p.nextToken()
		}
	}

	// 解析 import 声明
	for p.curTokenIs(lexer.TOKEN_IMPORT) {
		imp := p.parseImportDecl()
		if imp != nil {
			file.Imports = append(file.Imports, imp)
		}
	}

	// 解析其他语句
	for !p.curTokenIs(lexer.TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			file.Statements = append(file.Statements, stmt)
		}
		p.nextToken()
	}

	return file
}

// parseImportDecl 解析 import 声明
func (p *Parser) parseImportDecl() *ImportDecl {
	decl := &ImportDecl{Token: p.curToken}
	p.nextToken()

	if p.curTokenIs(lexer.TOKEN_LPAREN) {
		// 多行导入
		p.nextToken()
		for !p.curTokenIs(lexer.TOKEN_RPAREN) && !p.curTokenIs(lexer.TOKEN_EOF) {
			spec := p.parseImportSpec()
			if spec != nil {
				decl.Specs = append(decl.Specs, spec)
			}
			p.nextToken()
		}
		p.nextToken() // 消费 )
	} else {
		// 单行导入
		spec := p.parseImportSpec()
		if spec != nil {
			decl.Specs = append(decl.Specs, spec)
		}
		p.nextToken() // 消费最后一个 token，准备下一个 import 或语句
	}

	return decl
}

// parseImportSpec 解析单个导入项
// 支持的语法:
// - import "com.company.demo.models.User"  (tugo 风格，最后一截是类型名)
// - import Helper "com.company.demo.utils.StringHelper"  (带别名)
// - import "fmt" from golang  (Go 标准库)
// - import "tugo.lang.Str"  (tugo 标准库)
func (p *Parser) parseImportSpec() *ImportSpec {
	spec := &ImportSpec{}

	// 检查是否有别名
	if p.curTokenIs(lexer.TOKEN_IDENT) && !p.curTokenIs(lexer.TOKEN_FROM) {
		spec.Alias = p.curToken.Literal
		p.nextToken()
	} else if p.curTokenIs(lexer.TOKEN_DOT) {
		spec.Alias = "."
		p.nextToken()
	}

	if p.curTokenIs(lexer.TOKEN_STRING) {
		spec.Path = p.curToken.Literal
		// 去掉引号
		if len(spec.Path) >= 2 {
			spec.Path = spec.Path[1 : len(spec.Path)-1]
		}

		// 检查是否有 from golang
		if p.peekTokenIs(lexer.TOKEN_FROM) {
			p.nextToken() // 消费 from
			p.nextToken() // 消费 golang
			if p.curTokenIs(lexer.TOKEN_IDENT) && p.curToken.Literal == "golang" {
				spec.FromGo = true
				// Go 标准库，Path 就是包名，如 "fmt"
				spec.PkgPath = spec.Path
				spec.PkgName = spec.Path
			}
		} else {
			// 解析 tugo 风格的导入路径
			p.parseImportPath(spec)
		}
	}

	return spec
}

// parseImportPath 解析 tugo 风格的导入路径
// "com.company.demo.models.User" -> PkgPath="com.company.demo.models", TypeName="User", PkgName="models"
// "tugo.lang.Str" -> FromTugo=true, PkgPath="tugo.lang", TypeName="Str", PkgName="lang"
func (p *Parser) parseImportPath(spec *ImportSpec) {
	path := spec.Path

	// 检查是否是 tugo 标准库
	if len(path) >= 5 && path[:5] == "tugo." {
		spec.FromTugo = true
	}

	// 找到最后一个点
	lastDot := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			lastDot = i
			break
		}
	}

	if lastDot > 0 {
		spec.PkgPath = path[:lastDot]
		spec.TypeName = path[lastDot+1:]

		// 找到包名（PkgPath 的最后一截）
		secondLastDot := -1
		for i := lastDot - 1; i >= 0; i-- {
			if path[i] == '.' {
				secondLastDot = i
				break
			}
		}
		if secondLastDot >= 0 {
			spec.PkgName = path[secondLastDot+1 : lastDot]
		} else {
			spec.PkgName = path[:lastDot]
		}
	} else {
		// 没有点，整个就是包名（可能是 Go 标准库或单层包）
		spec.PkgPath = path
		spec.PkgName = path
	}
}

// parseStatement 解析语句
func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case lexer.TOKEN_PUBLIC:
		return p.parsePublicDecl()
	case lexer.TOKEN_PRIVATE:
		return p.parsePrivateDecl()
	case lexer.TOKEN_PROTECTED:
		return p.parseProtectedDecl()
	case lexer.TOKEN_FUNC:
		return p.parseFuncDecl(false, "private")
	case lexer.TOKEN_STRUCT:
		return p.parseStructDecl(false)
	case lexer.TOKEN_CLASS:
		return p.parseClassDecl(false)
	case lexer.TOKEN_ABSTRACT:
		return p.parseAbstractClassDecl(false)
	case lexer.TOKEN_STATIC:
		return p.parseStaticClassDecl(false)
	case lexer.TOKEN_TYPE:
		return p.parseTypeDecl(false)
	case lexer.TOKEN_INTERFACE:
		return p.parseInterfaceDecl(false)
	case lexer.TOKEN_VAR:
		return p.parseVarDecl()
	case lexer.TOKEN_CONST:
		return p.parseConstDecl()
	case lexer.TOKEN_RETURN:
		return p.parseReturnStmt()
	case lexer.TOKEN_IF:
		return p.parseIfStmt()
	case lexer.TOKEN_FOR:
		return p.parseForStmt()
	case lexer.TOKEN_SWITCH:
		return p.parseSwitchStmt()
	case lexer.TOKEN_SELECT:
		return p.parseSelectStmt()
	case lexer.TOKEN_GO:
		return p.parseGoStmt()
	case lexer.TOKEN_DEFER:
		return p.parseDeferStmt()
	case lexer.TOKEN_BREAK:
		return p.parseBreakStmt()
	case lexer.TOKEN_CONTINUE:
		return p.parseContinueStmt()
	case lexer.TOKEN_FALLTHROUGH:
		return p.parseFallthroughStmt()
	case lexer.TOKEN_TRY:
		return p.parseTryStmt()
	case lexer.TOKEN_THROW:
		return p.parseThrowStmt()
	case lexer.TOKEN_LBRACE:
		return p.parseBlockStmt()
	default:
		return p.parseExpressionStatement()
	}
}

// parsePublicDecl 解析 public 声明
func (p *Parser) parsePublicDecl() Statement {
	p.nextToken()
	switch p.curToken.Type {
	case lexer.TOKEN_FUNC:
		return p.parseFuncDecl(true, "public")
	case lexer.TOKEN_STRUCT:
		return p.parseStructDecl(true)
	case lexer.TOKEN_CLASS:
		return p.parseClassDecl(true)
	case lexer.TOKEN_ABSTRACT:
		return p.parseAbstractClassDecl(true)
	case lexer.TOKEN_STATIC:
		return p.parseStaticClassDecl(true)
	case lexer.TOKEN_TYPE:
		return p.parseTypeDecl(true)
	case lexer.TOKEN_INTERFACE:
		return p.parseInterfaceDecl(true)
	case lexer.TOKEN_VAR:
		return p.parseVarDecl() // public var
	default:
		p.addError("expected func, struct, class, abstract, static, type or interface after public")
		return nil
	}
}

// parsePrivateDecl 解析 private 声明
func (p *Parser) parsePrivateDecl() Statement {
	p.nextToken()
	switch p.curToken.Type {
	case lexer.TOKEN_STATIC:
		return p.parseStaticDecl("private")
	case lexer.TOKEN_VAR:
		return p.parseVarDecl()
	case lexer.TOKEN_FUNC:
		return p.parseFuncDecl(false, "private")
	default:
		p.addError("expected static, var or func after private")
		return nil
	}
}

// parseProtectedDecl 解析 protected 声明
func (p *Parser) parseProtectedDecl() Statement {
	p.nextToken()
	switch p.curToken.Type {
	case lexer.TOKEN_FUNC:
		return p.parseFuncDecl(false, "protected")
	case lexer.TOKEN_VAR:
		return p.parseVarDecl()
	default:
		p.addError("expected func or var after protected")
		return nil
	}
}

// parseStaticDecl 解析 static 声明（在 class 内部）
func (p *Parser) parseStaticDecl(visibility string) Statement {
	p.nextToken()
	// 处理 static 后面的类型和变量名
	return p.parseVarDecl()
}

// parseFuncDecl 解析函数声明
func (p *Parser) parseFuncDecl(public bool, visibility string) *FuncDecl {
	decl := &FuncDecl{Token: p.curToken, Public: public}
	p.nextToken()

	// 解析接收者
	if p.curTokenIs(lexer.TOKEN_LPAREN) {
		decl.Receiver = p.parseReceiver()
	}

	// 解析函数名
	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected function name")
		return nil
	}
	decl.Name = p.curToken.Literal
	p.nextToken()

	// 解析参数列表
	if !p.curTokenIs(lexer.TOKEN_LPAREN) {
		p.addError("expected ( after function name")
		return nil
	}
	decl.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 解析返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		decl.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if !p.peekTokenIs(lexer.TOKEN_LBRACE) {
		// 单个返回值类型
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			decl.Results = []*Field{{Type: typ}}
		}
	}

	// 检查是否有 ! 标记（errable）
	if p.peekTokenIs(lexer.TOKEN_NOT) {
		decl.Errable = true
		p.nextToken()
	}

	// 解析函数体
	if p.peekTokenIs(lexer.TOKEN_LBRACE) {
		p.nextToken()
		decl.Body = p.parseBlockStmt()
	}

	return decl
}

// parseReceiver 解析方法接收者
func (p *Parser) parseReceiver() *Field {
	p.nextToken() // 跳过 (
	field := &Field{}

	if p.curTokenIs(lexer.TOKEN_IDENT) {
		field.Name = p.curToken.Literal
		p.nextToken()
	}

	field.Type = p.parseType()

	if !p.curTokenIs(lexer.TOKEN_RPAREN) {
		p.expectPeek(lexer.TOKEN_RPAREN)
	}
	p.nextToken()

	return field
}

// parseFieldList 解析字段列表（参数或返回值）
func (p *Parser) parseFieldList(end lexer.TokenType) []*Field {
	var fields []*Field
	p.nextToken() // 跳过 (

	for !p.curTokenIs(end) && !p.curTokenIs(lexer.TOKEN_EOF) {
		field := p.parseField()
		if field != nil {
			fields = append(fields, field)
		}

		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if p.peekTokenIs(end) {
		p.nextToken()
	}

	return fields
}

// parseField 解析单个字段（参数或返回值）
func (p *Parser) parseField() *Field {
	field := &Field{}

	// 解析名称
	if p.curTokenIs(lexer.TOKEN_IDENT) && (p.peekTokenIs(lexer.TOKEN_COLON) || p.peekTokenIs(lexer.TOKEN_IDENT) || p.peekTokenIs(lexer.TOKEN_ASTERISK) || p.peekTokenIs(lexer.TOKEN_LBRACKET) || p.peekTokenIs(lexer.TOKEN_MAP) || p.peekTokenIs(lexer.TOKEN_CHAN) || p.peekTokenIs(lexer.TOKEN_FUNC) || p.peekTokenIs(lexer.TOKEN_INTERFACE) || p.peekTokenIs(lexer.TOKEN_STRUCT) || p.peekTokenIs(lexer.TOKEN_ELLIPSIS)) {
		field.Name = p.curToken.Literal
		p.nextToken()

		// 跳过可选的冒号（tugo 风格: name:type）
		if p.curTokenIs(lexer.TOKEN_COLON) {
			p.nextToken()
		}
	}

	// 解析类型
	field.Type = p.parseType()

	// 解析默认值
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		field.DefaultValue = p.parseExpression(LOWEST)
	}

	return field
}

// parseStructDecl 解析结构体声明
func (p *Parser) parseStructDecl(public bool) *StructDecl {
	decl := &StructDecl{Token: p.curToken, Public: public}
	p.nextToken()

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected struct name")
		return nil
	}
	decl.Name = p.curToken.Literal

	// 解析 implements 接口列表
	if p.peekTokenIs(lexer.TOKEN_IMPLEMENTS) {
		p.nextToken() // 移动到 implements
		p.nextToken() // 移动到第一个接口名
		for {
			if !p.curTokenIs(lexer.TOKEN_IDENT) {
				p.addError("expected interface name after implements")
				return nil
			}
			decl.Implements = append(decl.Implements, p.curToken.Literal)
			if !p.peekTokenIs(lexer.TOKEN_COMMA) {
				break
			}
			p.nextToken() // 跳过逗号
			p.nextToken() // 移动到下一个接口名
		}
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	// 解析结构体成员
	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		member := p.parseStructMember()
		if member != nil {
			switch m := member.(type) {
			case *StructField:
				decl.Fields = append(decl.Fields, m)
			case *ClassMethod:
				if m.Name == "init" {
					decl.InitMethod = m
				} else {
					decl.Methods = append(decl.Methods, m)
				}
			case string:
				// 嵌入类型
				decl.Embeds = append(decl.Embeds, m)
			}
		}
		p.nextToken()
	}

	return decl
}

// parseStructMember 解析结构体成员（字段、方法、嵌入）
func (p *Parser) parseStructMember() interface{} {
	visibility := "private" // 默认 private

	// 检查可见性修饰符
	if p.curTokenIs(lexer.TOKEN_PUBLIC) {
		visibility = "public"
		p.nextToken()
	} else if p.curTokenIs(lexer.TOKEN_PRIVATE) {
		visibility = "private"
		p.nextToken()
	}

	// 解析成员
	switch p.curToken.Type {
	case lexer.TOKEN_VAR:
		return p.parseStructFieldWithVar(visibility)
	case lexer.TOKEN_FUNC:
		return p.parseStructMethod(visibility)
	case lexer.TOKEN_IDENT:
		// 可能是嵌入类型或字段
		return p.parseStructFieldOrEmbed(visibility)
	default:
		return nil
	}
}

// parseStructFieldWithVar 解析带 var 的结构体字段
func (p *Parser) parseStructFieldWithVar(visibility string) *StructField {
	field := &StructField{Visibility: visibility, Public: visibility == "public"}
	p.nextToken() // 跳过 var

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		return nil
	}
	field.Name = p.curToken.Literal
	p.nextToken()

	// 解析类型
	field.Type = p.parseType()

	// 解析 tag
	if p.peekTokenIs(lexer.TOKEN_STRING) {
		p.nextToken()
		field.Tag = p.curToken.Literal
	}

	return field
}

// parseStructFieldOrEmbed 解析字段或嵌入类型
func (p *Parser) parseStructFieldOrEmbed(visibility string) interface{} {
	first := p.curToken.Literal

	// 检查下一个 token
	if p.peekTokenIs(lexer.TOKEN_RBRACE) || p.peekTokenIs(lexer.TOKEN_PUBLIC) ||
		p.peekTokenIs(lexer.TOKEN_PRIVATE) || p.peekTokenIs(lexer.TOKEN_FUNC) ||
		p.peekTokenIs(lexer.TOKEN_VAR) || p.peekTokenIs(lexer.TOKEN_IDENT) && isTypeName(first) {
		// 只有类型名，这是嵌入（首字母大写的标识符）
		if !p.peekTokenIs(lexer.TOKEN_IDENT) {
			return first
		}
	}

	if p.peekTokenIs(lexer.TOKEN_IDENT) {
		// "name type" 形式 - 第一个是字段名，第二个是类型
		p.nextToken()
		typeName := p.curToken.Literal
		field := &StructField{
			Visibility: visibility,
			Public:     visibility == "public",
			Name:       first,
			Type:       &Identifier{Token: lexer.Token{Type: lexer.TOKEN_IDENT, Literal: typeName}, Value: typeName},
		}
		// 解析 tag
		if p.peekTokenIs(lexer.TOKEN_STRING) {
			p.nextToken()
			field.Tag = p.curToken.Literal
		}
		return field
	}

	// 只有一个标识符，可能是嵌入
	return first
}

// isTypeName 检查是否是类型名（首字母大写表示可能是嵌入类型）
func isTypeName(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// parseStructFieldOldStyle 解析旧式字段（保留以备用）
func (p *Parser) parseStructFieldOldStyle(visibility string) *StructField {
	first := p.curToken.Literal
	p.nextToken()
	field := &StructField{
		Visibility: visibility,
		Public:     visibility == "public",
		Name:       first,
		Type:       p.parseType(),
	}
	// 解析 tag
	if p.peekTokenIs(lexer.TOKEN_STRING) {
		p.nextToken()
		field.Tag = p.curToken.Literal
	}
	return field
}

// parseStructMethod 解析结构体方法
func (p *Parser) parseStructMethod(visibility string) *ClassMethod {
	method := &ClassMethod{Token: p.curToken, Visibility: visibility}
	p.nextToken() // 跳过 func

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected method name")
		return nil
	}
	method.Name = p.curToken.Literal
	p.nextToken()

	// 解析参数
	if !p.curTokenIs(lexer.TOKEN_LPAREN) {
		p.addError("expected ( after method name")
		return nil
	}
	method.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 解析返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		method.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if !p.peekTokenIs(lexer.TOKEN_LBRACE) && !p.peekTokenIs(lexer.TOKEN_RBRACE) && !p.peekTokenIs(lexer.TOKEN_EOF) {
		// 单个返回值类型
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			method.Results = []*Field{{Type: typ}}
		}
	}

	// 检查是否有 ! 标记（errable）
	if p.peekTokenIs(lexer.TOKEN_NOT) {
		method.Errable = true
		p.nextToken()
	}

	// 解析方法体
	if p.peekTokenIs(lexer.TOKEN_LBRACE) {
		p.nextToken()
		method.Body = p.parseBlockStmt()
	}

	return method
}

// parseClassDecl 解析类声明
func (p *Parser) parseClassDecl(public bool) *ClassDecl {
	return p.parseClassDeclFull(public, false, false)
}

func (p *Parser) parseAbstractClassDecl(public bool) *ClassDecl {
	// 已经在 abstract 关键字上，跳到 class
	p.nextToken()
	if !p.curTokenIs(lexer.TOKEN_CLASS) {
		p.addError("expected 'class' after 'abstract'")
		return nil
	}
	return p.parseClassDeclFull(public, true, false)
}

func (p *Parser) parseStaticClassDecl(public bool) *ClassDecl {
	// 已经在 static 关键字上，跳到 class
	p.nextToken()
	if !p.curTokenIs(lexer.TOKEN_CLASS) {
		p.addError("expected 'class' after 'static'")
		return nil
	}
	return p.parseClassDeclFull(public, false, true)
}

func (p *Parser) parseClassDeclFull(public bool, abstract bool, static bool) *ClassDecl {
	decl := &ClassDecl{Token: p.curToken, Public: public, Abstract: abstract, Static: static}
	p.nextToken()

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected class name")
		return nil
	}
	decl.Name = p.curToken.Literal

	// 静态类不能有 extends
	if p.peekTokenIs(lexer.TOKEN_EXTENDS) {
		if static {
			p.addError("static class cannot extend another class")
			return nil
		}
		p.nextToken() // 移动到 extends
		p.nextToken() // 移动到父类名
		if !p.curTokenIs(lexer.TOKEN_IDENT) {
			p.addError("expected parent class name after extends")
			return nil
		}
		decl.Extends = p.curToken.Literal
	}

	// 静态类不能 implements
	if p.peekTokenIs(lexer.TOKEN_IMPLEMENTS) {
		if static {
			p.addError("static class cannot implement interfaces")
			return nil
		}
		p.nextToken() // 移动到 implements
		p.nextToken() // 移动到第一个接口名
		for {
			if !p.curTokenIs(lexer.TOKEN_IDENT) {
				p.addError("expected interface name after implements")
				return nil
			}
			decl.Implements = append(decl.Implements, p.curToken.Literal)
			if !p.peekTokenIs(lexer.TOKEN_COMMA) {
				break
			}
			p.nextToken() // 跳过逗号
			p.nextToken() // 移动到下一个接口名
		}
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	// 解析类成员
	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		member := p.parseClassMember()
		if member != nil {
			switch m := member.(type) {
			case *ClassField:
				decl.Fields = append(decl.Fields, m)
			case *ClassMethod:
				if m.Name == "init" {
					decl.InitMethod = m
				} else if m.Abstract {
					decl.AbstractMethods = append(decl.AbstractMethods, m)
				} else {
					decl.Methods = append(decl.Methods, m)
				}
			}
		}
		p.nextToken()
	}

	return decl
}

// parseClassMember 解析类成员（字段或方法）
func (p *Parser) parseClassMember() interface{} {
	visibility := "private" // 默认 private

	// 检查可见性修饰符
	if p.curTokenIs(lexer.TOKEN_PUBLIC) {
		visibility = "public"
		p.nextToken()
	} else if p.curTokenIs(lexer.TOKEN_PRIVATE) {
		visibility = "private"
		p.nextToken()
	} else if p.curTokenIs(lexer.TOKEN_PROTECTED) {
		visibility = "protected"
		p.nextToken()
	}

	// 检查是否是 static
	isStatic := false
	if p.curTokenIs(lexer.TOKEN_STATIC) {
		isStatic = true
		p.nextToken()
	}

	// 检查是否是 abstract
	isAbstract := false
	if p.curTokenIs(lexer.TOKEN_ABSTRACT) {
		isAbstract = true
		p.nextToken()
	}

	// 解析成员
	switch p.curToken.Type {
	case lexer.TOKEN_VAR:
		return p.parseClassField(visibility, isStatic)
	case lexer.TOKEN_FUNC:
		return p.parseClassMethodWithAbstract(visibility, isStatic, isAbstract)
	case lexer.TOKEN_IDENT:
		// 可能是类型声明，如 "string title" 或 "name string"
		return p.parseClassFieldShort(visibility, isStatic)
	default:
		return nil
	}
}

// parseClassField 解析类字段 (var name type = value)
func (p *Parser) parseClassField(visibility string, isStatic bool) *ClassField {
	field := &ClassField{Visibility: visibility, Static: isStatic}
	p.nextToken() // 跳过 var

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		return nil
	}
	field.Name = p.curToken.Literal
	p.nextToken()

	// 解析类型
	field.Type = p.parseType()

	// 解析默认值
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		field.Value = p.parseExpression(LOWEST)
	}

	return field
}

// parseClassFieldShort 解析简短形式的类字段
// 支持两种格式:
// - "name type" 形式: public role_id int
// - "type name" 形式: public string role_name (如果第一个是已知类型)
func (p *Parser) parseClassFieldShort(visibility string, isStatic bool) *ClassField {
	field := &ClassField{Visibility: visibility, Static: isStatic}

	// 第一个 token 可能是类型或名称
	first := p.curToken.Literal
	firstToken := p.curToken
	p.nextToken()

	if p.curTokenIs(lexer.TOKEN_IDENT) {
		// 有两个标识符，需要判断格式
		second := p.curToken.Literal

		// 检查第一个是否是已知的基本类型
		if isBasicType(first) {
			// "type name" 形式: string name
			field.Type = &Identifier{Token: firstToken, Value: first}
			field.Name = second
		} else {
			// "name type" 形式: name string
			field.Name = first
			field.Type = &Identifier{Token: p.curToken, Value: second}
		}
	} else {
		// 只有一个标识符，可能是复杂类型
		field.Name = first
		field.Type = p.parseType()
	}

	// 解析默认值
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		field.Value = p.parseExpression(LOWEST)
	}

	return field
}

// isBasicType 检查是否是基本类型
func isBasicType(name string) bool {
	basicTypes := map[string]bool{
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true,
		"string": true, "bool": true, "byte": true, "rune": true,
		"complex64": true, "complex128": true,
		"error": true, "any": true,
	}
	return basicTypes[name]
}

// parseClassMethod 解析类方法
func (p *Parser) parseClassMethod(visibility string, isStatic bool) *ClassMethod {
	return p.parseClassMethodWithAbstract(visibility, isStatic, false)
}

func (p *Parser) parseClassMethodWithAbstract(visibility string, isStatic bool, isAbstract bool) *ClassMethod {
	method := &ClassMethod{Token: p.curToken, Visibility: visibility, Static: isStatic, Abstract: isAbstract}
	p.nextToken()

	// 方法名
	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected method name")
		return nil
	}
	method.Name = p.curToken.Literal
	p.nextToken()

	// 参数列表
	if !p.curTokenIs(lexer.TOKEN_LPAREN) {
		p.addError("expected ( after method name")
		return nil
	}
	method.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		method.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if !p.peekTokenIs(lexer.TOKEN_LBRACE) && !p.peekTokenIs(lexer.TOKEN_RBRACE) && !p.peekTokenIs(lexer.TOKEN_EOF) && !p.peekTokenIs(lexer.TOKEN_NOT) {
		// 单个返回值类型
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			method.Results = []*Field{{Type: typ}}
		}
	}

	// 检查是否有 ! 标记（errable）
	if p.peekTokenIs(lexer.TOKEN_NOT) {
		method.Errable = true
		p.nextToken()
	}

	// 方法体（抽象方法没有方法体）
	if !isAbstract && p.peekTokenIs(lexer.TOKEN_LBRACE) {
		p.nextToken()
		method.Body = p.parseBlockStmt()
	}

	return method
}

// parseStructField 解析结构体字段
func (p *Parser) parseStructField() *StructField {
	field := &StructField{}

	// 检查是否 public
	if p.curTokenIs(lexer.TOKEN_PUBLIC) {
		field.Public = true
		p.nextToken()
	}

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		return nil
	}
	field.Name = p.curToken.Literal
	p.nextToken()

	// 跳过可选的冒号
	if p.curTokenIs(lexer.TOKEN_COLON) {
		p.nextToken()
	}

	field.Type = p.parseType()

	// 解析 tag
	if p.peekTokenIs(lexer.TOKEN_STRING) {
		p.nextToken()
		field.Tag = p.curToken.Literal
	}

	return field
}

// parseInterfaceDecl 解析接口声明
func (p *Parser) parseInterfaceDecl(public bool) *InterfaceDecl {
	decl := &InterfaceDecl{Token: p.curToken, Public: public}
	p.nextToken()

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected interface name")
		return nil
	}
	decl.Name = p.curToken.Literal

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	// 解析方法签名
	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		sig := p.parseFuncSignature()
		if sig != nil {
			decl.Methods = append(decl.Methods, sig)
		}
		p.nextToken()
	}

	return decl
}

// parseFuncSignature 解析函数签名（用于接口）
func (p *Parser) parseFuncSignature() *FuncSignature {
	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		return nil
	}

	sig := &FuncSignature{Name: p.curToken.Literal}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	sig.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 解析返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		// 多个返回值
		p.nextToken()
		sig.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if p.peekTokenIs(lexer.TOKEN_IDENT) || p.peekTokenIs(lexer.TOKEN_ASTERISK) ||
		p.peekTokenIs(lexer.TOKEN_LBRACKET) || p.peekTokenIs(lexer.TOKEN_MAP) ||
		p.peekTokenIs(lexer.TOKEN_CHAN) || p.peekTokenIs(lexer.TOKEN_FUNC) ||
		p.peekTokenIs(lexer.TOKEN_INTERFACE) || p.peekTokenIs(lexer.TOKEN_STRUCT) {
		// 单个返回值类型
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			sig.Results = []*Field{{Type: typ}}
		}
	}

	// 检查是否有 ! 标记（errable）
	if p.peekTokenIs(lexer.TOKEN_NOT) {
		sig.Errable = true
		p.nextToken()
	}

	return sig
}

// parseTypeDecl 解析类型声明
func (p *Parser) parseTypeDecl(public bool) *TypeDecl {
	decl := &TypeDecl{Token: p.curToken, Public: public}
	p.nextToken()

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		p.addError("expected type name")
		return nil
	}
	decl.Name = p.curToken.Literal
	p.nextToken()

	decl.Type = p.parseType()

	return decl
}

// parseVarDecl 解析变量声明
func (p *Parser) parseVarDecl() *VarDecl {
	decl := &VarDecl{Token: p.curToken}
	p.nextToken()

	// 解析变量名
	for p.curTokenIs(lexer.TOKEN_IDENT) {
		decl.Names = append(decl.Names, p.curToken.Literal)
		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	// 解析类型
	if !p.peekTokenIs(lexer.TOKEN_ASSIGN) && !p.peekTokenIs(lexer.TOKEN_EOF) && !p.peekTokenIs(lexer.TOKEN_SEMICOLON) {
		p.nextToken()
		decl.Type = p.parseType()
	}

	// 解析初始值
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		decl.Value = p.parseExpression(LOWEST)
	}

	return decl
}

// parseConstDecl 解析常量声明
func (p *Parser) parseConstDecl() *ConstDecl {
	decl := &ConstDecl{Token: p.curToken}
	p.nextToken()

	// 解析常量名
	for p.curTokenIs(lexer.TOKEN_IDENT) {
		decl.Names = append(decl.Names, p.curToken.Literal)
		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	// 解析类型
	if !p.peekTokenIs(lexer.TOKEN_ASSIGN) && !p.peekTokenIs(lexer.TOKEN_EOF) {
		p.nextToken()
		decl.Type = p.parseType()
	}

	// 解析值
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		p.nextToken()
		p.nextToken()
		decl.Value = p.parseExpression(LOWEST)
	}

	return decl
}

// parseReturnStmt 解析 return 语句
func (p *Parser) parseReturnStmt() *ReturnStmt {
	stmt := &ReturnStmt{Token: p.curToken}

	if p.peekTokenIs(lexer.TOKEN_RBRACE) || p.peekTokenIs(lexer.TOKEN_SEMICOLON) || p.peekTokenIs(lexer.TOKEN_EOF) {
		return stmt
	}

	p.nextToken()
	stmt.Values = p.parseExpressionList()

	return stmt
}

// parseIfStmt 解析 if 语句
func (p *Parser) parseIfStmt() *IfStmt {
	stmt := &IfStmt{Token: p.curToken}
	p.nextToken()

	// 解析条件（可能带有初始化语句）
	expr := p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.TOKEN_SEMICOLON) {
		// 有初始化语句
		stmt.Init = &ExpressionStmt{Expression: expr}
		p.nextToken()
		p.nextToken()
		stmt.Condition = p.parseExpression(LOWEST)
	} else {
		stmt.Condition = expr
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	stmt.Consequence = p.parseBlockStmt()

	if p.peekTokenIs(lexer.TOKEN_ELSE) {
		p.nextToken()
		if p.peekTokenIs(lexer.TOKEN_IF) {
			p.nextToken()
			stmt.Alternative = p.parseIfStmt()
		} else if p.expectPeek(lexer.TOKEN_LBRACE) {
			stmt.Alternative = p.parseBlockStmt()
		}
	}

	return stmt
}

// parseForStmt 解析 for 语句
func (p *Parser) parseForStmt() Statement {
	token := p.curToken
	p.nextToken()

	// 检查是否是 range 循环
	// for 后面直接跟 { 是无限循环
	if p.curTokenIs(lexer.TOKEN_LBRACE) {
		return &ForStmt{Token: token, Body: p.parseBlockStmt()}
	}

	// 尝试解析 for range
	// 形式: for k, v := range x { } 或 for range x { }
	startPos := p.curToken

	// 如果是 range 关键字开头
	if p.curTokenIs(lexer.TOKEN_RANGE) {
		p.nextToken()
		x := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.TOKEN_LBRACE) {
			return nil
		}
		return &RangeStmt{Token: token, X: x, Body: p.parseBlockStmt()}
	}

	// 解析第一个表达式
	firstExpr := p.parseExpression(LOWEST)

	// 检查是否是 range 循环
	if p.peekTokenIs(lexer.TOKEN_DEFINE) || p.peekTokenIs(lexer.TOKEN_ASSIGN) {
		// 可能是 k, v := range x 或 k, v = range x
		var names []Expression
		names = append(names, firstExpr)

		// 收集所有变量名
		for p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
			names = append(names, p.parseExpression(LOWEST))
		}

		if p.peekTokenIs(lexer.TOKEN_DEFINE) || p.peekTokenIs(lexer.TOKEN_ASSIGN) {
			p.nextToken()
			p.nextToken()

			if p.curTokenIs(lexer.TOKEN_RANGE) {
				p.nextToken()
				x := p.parseExpression(LOWEST)
				if !p.expectPeek(lexer.TOKEN_LBRACE) {
					return nil
				}
				rangeStmt := &RangeStmt{Token: token, X: x, Body: p.parseBlockStmt()}
				if len(names) > 0 {
					rangeStmt.Key = names[0]
				}
				if len(names) > 1 {
					rangeStmt.Value = names[1]
				}
				return rangeStmt
			}
		}
	}

	// 普通 for 循环
	_ = startPos
	forStmt := &ForStmt{Token: token}

	// 检查是否有分号（三段式 for 循环）
	if p.peekTokenIs(lexer.TOKEN_SEMICOLON) {
		// 三段式 for 循环
		forStmt.Init = &ExpressionStmt{Expression: firstExpr}
		p.nextToken() // 跳过 ;
		p.nextToken()

		if !p.curTokenIs(lexer.TOKEN_SEMICOLON) {
			forStmt.Condition = p.parseExpression(LOWEST)
		}

		if !p.expectPeek(lexer.TOKEN_SEMICOLON) {
			return nil
		}
		p.nextToken()

		if !p.curTokenIs(lexer.TOKEN_LBRACE) {
			forStmt.Post = &ExpressionStmt{Expression: p.parseExpression(LOWEST)}
		}
	} else {
		// 只有条件的 for 循环
		forStmt.Condition = firstExpr
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	forStmt.Body = p.parseBlockStmt()

	return forStmt
}

// parseSwitchStmt 解析 switch 语句
func (p *Parser) parseSwitchStmt() *SwitchStmt {
	stmt := &SwitchStmt{Token: p.curToken}
	p.nextToken()

	// 解析初始化和 tag
	if !p.curTokenIs(lexer.TOKEN_LBRACE) {
		expr := p.parseExpression(LOWEST)
		if p.peekTokenIs(lexer.TOKEN_SEMICOLON) {
			stmt.Init = &ExpressionStmt{Expression: expr}
			p.nextToken()
			p.nextToken()
			if !p.curTokenIs(lexer.TOKEN_LBRACE) {
				stmt.Tag = p.parseExpression(LOWEST)
			}
		} else {
			stmt.Tag = expr
		}
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	// 解析 case 子句
	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		clause := p.parseCaseClause()
		if clause != nil {
			stmt.Cases = append(stmt.Cases, clause)
		}
		p.nextToken()
	}

	return stmt
}

// parseCaseClause 解析 case 子句
func (p *Parser) parseCaseClause() *CaseClause {
	clause := &CaseClause{Token: p.curToken}

	if p.curTokenIs(lexer.TOKEN_CASE) {
		p.nextToken()
		clause.Exprs = p.parseExpressionList()
	} else if p.curTokenIs(lexer.TOKEN_DEFAULT) {
		// default case
	} else {
		return nil
	}

	if !p.expectPeek(lexer.TOKEN_COLON) {
		return nil
	}
	p.nextToken()

	// 解析 case 体
	for !p.curTokenIs(lexer.TOKEN_CASE) && !p.curTokenIs(lexer.TOKEN_DEFAULT) && !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			clause.Body = append(clause.Body, stmt)
		}
		p.nextToken()
	}

	return clause
}

// parseSelectStmt 解析 select 语句
func (p *Parser) parseSelectStmt() *SelectStmt {
	stmt := &SelectStmt{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	// 解析通信子句
	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		clause := p.parseCommClause()
		if clause != nil {
			stmt.Cases = append(stmt.Cases, clause)
		}
		p.nextToken()
	}

	return stmt
}

// parseCommClause 解析通信子句
func (p *Parser) parseCommClause() *CommClause {
	clause := &CommClause{Token: p.curToken}

	if p.curTokenIs(lexer.TOKEN_CASE) {
		p.nextToken()
		// 解析通信语句
		clause.Comm = p.parseStatement()
	} else if p.curTokenIs(lexer.TOKEN_DEFAULT) {
		// default case
	} else {
		return nil
	}

	if !p.expectPeek(lexer.TOKEN_COLON) {
		return nil
	}
	p.nextToken()

	// 解析 case 体
	for !p.curTokenIs(lexer.TOKEN_CASE) && !p.curTokenIs(lexer.TOKEN_DEFAULT) && !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			clause.Body = append(clause.Body, stmt)
		}
		p.nextToken()
	}

	return clause
}

// parseGoStmt 解析 go 语句
func (p *Parser) parseGoStmt() *GoStmt {
	stmt := &GoStmt{Token: p.curToken}
	p.nextToken()

	expr := p.parseExpression(LOWEST)
	if call, ok := expr.(*CallExpr); ok {
		stmt.Call = call
	}

	return stmt
}

// parseDeferStmt 解析 defer 语句
func (p *Parser) parseDeferStmt() *DeferStmt {
	stmt := &DeferStmt{Token: p.curToken}
	p.nextToken()

	expr := p.parseExpression(LOWEST)
	if call, ok := expr.(*CallExpr); ok {
		stmt.Call = call
	}

	return stmt
}

// parseBreakStmt 解析 break 语句
func (p *Parser) parseBreakStmt() *BreakStmt {
	stmt := &BreakStmt{Token: p.curToken}

	if p.peekTokenIs(lexer.TOKEN_IDENT) {
		p.nextToken()
		stmt.Label = p.curToken.Literal
	}

	return stmt
}

// parseContinueStmt 解析 continue 语句
func (p *Parser) parseContinueStmt() *ContinueStmt {
	stmt := &ContinueStmt{Token: p.curToken}

	if p.peekTokenIs(lexer.TOKEN_IDENT) {
		p.nextToken()
		stmt.Label = p.curToken.Literal
	}

	return stmt
}

// parseFallthroughStmt 解析 fallthrough 语句
func (p *Parser) parseFallthroughStmt() *FallthroughStmt {
	return &FallthroughStmt{Token: p.curToken}
}

// parseTryStmt 解析 try-catch 语句
func (p *Parser) parseTryStmt() *TryStmt {
	stmt := &TryStmt{Token: p.curToken}

	// 解析 try 块
	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStmt()

	// 解析 catch 子句
	if p.peekTokenIs(lexer.TOKEN_CATCH) {
		p.nextToken()
		stmt.Catch = p.parseCatchClause()
	}

	// TODO: 未来可以添加 finally 子句支持

	return stmt
}

// parseCatchClause 解析 catch 子句
func (p *Parser) parseCatchClause() *CatchClause {
	clause := &CatchClause{Token: p.curToken}

	// 解析异常参数名
	if p.peekTokenIs(lexer.TOKEN_IDENT) {
		p.nextToken()
		clause.Param = p.curToken.Literal
	}

	// 解析 catch 块
	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	clause.Body = p.parseBlockStmt()

	return clause
}

// parseThrowStmt 解析 throw 语句
func (p *Parser) parseThrowStmt() *ThrowStmt {
	stmt := &ThrowStmt{Token: p.curToken}

	if p.peekTokenIs(lexer.TOKEN_RBRACE) || p.peekTokenIs(lexer.TOKEN_SEMICOLON) || p.peekTokenIs(lexer.TOKEN_EOF) {
		p.addError("throw statement requires an expression")
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

// parseBlockStmt 解析代码块
func (p *Parser) parseBlockStmt() *BlockStmt {
	block := &BlockStmt{Token: p.curToken}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// parseExpressionStatement 解析表达式语句
func (p *Parser) parseExpressionStatement() Statement {
	expr := p.parseExpression(LOWEST)
	if expr == nil {
		return nil
	}

	// 检查是否是逗号分隔的表达式列表（用于多变量声明/赋值）
	if p.peekTokenIs(lexer.TOKEN_COMMA) {
		exprs := []Expression{expr}
		for p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken() // 跳过逗号
			p.nextToken() // 移到下一个表达式
			nextExpr := p.parseExpression(LOWEST)
			if nextExpr == nil {
				break
			}
			exprs = append(exprs, nextExpr)
		}
		
		// 现在检查是否是 := 或赋值
		if p.peekTokenIs(lexer.TOKEN_DEFINE) {
			// 多变量短声明
			return p.parseMultiShortVarDecl(exprs)
		}
		
		if p.peekTokenIs(lexer.TOKEN_ASSIGN) || p.peekTokenIs(lexer.TOKEN_PLUS_ASSIGN) ||
			p.peekTokenIs(lexer.TOKEN_MINUS_ASSIGN) || p.peekTokenIs(lexer.TOKEN_ASTERISK_ASSIGN) ||
			p.peekTokenIs(lexer.TOKEN_SLASH_ASSIGN) || p.peekTokenIs(lexer.TOKEN_PERCENT_ASSIGN) {
			return p.parseMultiAssignStmt(exprs)
		}
		
		// 否则是语法错误
		p.addError("unexpected comma in expression")
		return &ExpressionStmt{Expression: expr}
	}

	// 单个表达式，检查是否是短变量声明
	if p.peekTokenIs(lexer.TOKEN_DEFINE) {
		return p.parseShortVarDecl(expr)
	}

	// 检查是否是赋值语句
	if p.peekTokenIs(lexer.TOKEN_ASSIGN) || p.peekTokenIs(lexer.TOKEN_PLUS_ASSIGN) ||
		p.peekTokenIs(lexer.TOKEN_MINUS_ASSIGN) || p.peekTokenIs(lexer.TOKEN_ASTERISK_ASSIGN) ||
		p.peekTokenIs(lexer.TOKEN_SLASH_ASSIGN) || p.peekTokenIs(lexer.TOKEN_PERCENT_ASSIGN) {
		return p.parseAssignStmt(expr)
	}

	// 检查是否是自增/自减语句
	if p.peekTokenIs(lexer.TOKEN_INC) {
		p.nextToken()
		return &IncDecStmt{Token: p.curToken, X: expr, Inc: true}
	}
	if p.peekTokenIs(lexer.TOKEN_DEC) {
		p.nextToken()
		return &IncDecStmt{Token: p.curToken, X: expr, Inc: false}
	}

	// 检查是否是发送语句
	if p.peekTokenIs(lexer.TOKEN_ARROW) {
		p.nextToken()
		arrowToken := p.curToken
		p.nextToken()
		value := p.parseExpression(LOWEST)
		return &SendStmt{Token: arrowToken, Channel: expr, Value: value}
	}

	return &ExpressionStmt{Expression: expr}
}

// parseShortVarDecl 解析短变量声明（单变量）
func (p *Parser) parseShortVarDecl(firstExpr Expression) *ShortVarDecl {
	stmt := &ShortVarDecl{}

	// 提取变量名
	if ident, ok := firstExpr.(*Identifier); ok {
		stmt.Names = append(stmt.Names, ident.Value)
	}

	p.nextToken() // 跳过 :=
	stmt.Token = p.curToken
	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

// parseMultiShortVarDecl 解析多变量短声明
func (p *Parser) parseMultiShortVarDecl(exprs []Expression) *ShortVarDecl {
	stmt := &ShortVarDecl{}

	// 提取所有变量名
	for _, e := range exprs {
		if ident, ok := e.(*Identifier); ok {
			stmt.Names = append(stmt.Names, ident.Value)
		}
	}

	p.nextToken() // 跳过 :=
	stmt.Token = p.curToken
	p.nextToken()

	// 解析右侧的值（可能是单个函数调用返回多值，或多个表达式）
	firstValue := p.parseExpression(LOWEST)
	
	// 检查是否有更多值（逗号分隔）
	if p.peekTokenIs(lexer.TOKEN_COMMA) {
		// 多个值：a, b := 1, 2
		// 创建一个元组表达式来包含所有值
		values := []Expression{firstValue}
		for p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken() // 跳过逗号
			p.nextToken()
			values = append(values, p.parseExpression(LOWEST))
		}
		
		// 使用元组字面量来表示多个值
		stmt.Value = &ArrayLiteral{Elements: values} // 临时用 ArrayLiteral 表示多值
	} else {
		// 单个值（可能是返回多值的函数调用）
		stmt.Value = firstValue
	}

	return stmt
}

// parseAssignStmt 解析赋值语句（单变量）
func (p *Parser) parseAssignStmt(firstExpr Expression) *AssignStmt {
	stmt := &AssignStmt{}
	stmt.Left = append(stmt.Left, firstExpr)

	p.nextToken()
	stmt.Token = p.curToken
	p.nextToken()

	stmt.Right = append(stmt.Right, p.parseExpression(LOWEST))

	return stmt
}

// parseMultiAssignStmt 解析多变量赋值语句
func (p *Parser) parseMultiAssignStmt(exprs []Expression) *AssignStmt {
	stmt := &AssignStmt{}
	stmt.Left = exprs

	p.nextToken()
	stmt.Token = p.curToken
	p.nextToken()

	stmt.Right = p.parseExpressionList()

	return stmt
}

// parseExpressionList 解析表达式列表
func (p *Parser) parseExpressionList() []Expression {
	var exprs []Expression
	exprs = append(exprs, p.parseExpression(LOWEST))

	for p.peekTokenIs(lexer.TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		exprs = append(exprs, p.parseExpression(LOWEST))
	}

	return exprs
}

// 运算符优先级
const (
	_ int = iota
	LOWEST
	OR          // ||
	AND         // &&
	EQUALS      // == !=
	LESSGREATER // > < >= <=
	SUM         // + -
	PRODUCT     // * / %
	PREFIX      // -X !X &X *X
	CALL        // myFunc(X)
	INDEX       // array[index]
)

var precedences = map[lexer.TokenType]int{
	lexer.TOKEN_OR:       OR,
	lexer.TOKEN_AND:      AND,
	lexer.TOKEN_EQ:       EQUALS,
	lexer.TOKEN_NOT_EQ:   EQUALS,
	lexer.TOKEN_LT:       LESSGREATER,
	lexer.TOKEN_GT:       LESSGREATER,
	lexer.TOKEN_LT_EQ:    LESSGREATER,
	lexer.TOKEN_GT_EQ:    LESSGREATER,
	lexer.TOKEN_PLUS:     SUM,
	lexer.TOKEN_MINUS:    SUM,
	lexer.TOKEN_BIT_OR:   SUM,
	lexer.TOKEN_BIT_XOR:  SUM,
	lexer.TOKEN_ASTERISK: PRODUCT,
	lexer.TOKEN_SLASH:    PRODUCT,
	lexer.TOKEN_PERCENT:  PRODUCT,
	lexer.TOKEN_BIT_AND:  PRODUCT,
	lexer.TOKEN_SHL:      PRODUCT,
	lexer.TOKEN_SHR:      PRODUCT,
	lexer.TOKEN_LPAREN:       CALL,
	lexer.TOKEN_LBRACKET:    INDEX,
	lexer.TOKEN_DOT:          INDEX,
	lexer.TOKEN_DOUBLE_COLON: INDEX,
}

// peekPrecedence 获取下一个 token 的优先级
func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

// curPrecedence 获取当前 token 的优先级
func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

// parseExpression 解析表达式
func (p *Parser) parseExpression(precedence int) Expression {
	var left Expression

	switch p.curToken.Type {
	case lexer.TOKEN_IDENT:
		left = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_INT:
		left = &IntegerLiteral{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_FLOAT:
		left = &FloatLiteral{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_STRING:
		left = &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_CHAR:
		left = &CharLiteral{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_TRUE:
		left = &BoolLiteral{Token: p.curToken, Value: true}
	case lexer.TOKEN_FALSE:
		left = &BoolLiteral{Token: p.curToken, Value: false}
	case lexer.TOKEN_NIL:
		left = &NilLiteral{Token: p.curToken}
	case lexer.TOKEN_THIS:
		left = &ThisExpr{Token: p.curToken}
	case lexer.TOKEN_SELF:
		left = &SelfExpr{Token: p.curToken}
	case lexer.TOKEN_LPAREN:
		left = p.parseGroupedExpression()
	case lexer.TOKEN_LBRACKET:
		left = p.parseArrayOrSliceLiteral()
	case lexer.TOKEN_MAP:
		left = p.parseMapLiteral()
	case lexer.TOKEN_FUNC:
		left = p.parseFuncLiteral()
	case lexer.TOKEN_MINUS, lexer.TOKEN_NOT, lexer.TOKEN_BIT_AND, lexer.TOKEN_ASTERISK, lexer.TOKEN_BIT_XOR:
		left = p.parsePrefixExpression()
	case lexer.TOKEN_ARROW:
		left = p.parseReceiveExpression()
	case lexer.TOKEN_MAKE:
		left = p.parseMakeExpression()
	case lexer.TOKEN_NEW:
		left = p.parseNewExpression()
	case lexer.TOKEN_LEN:
		left = p.parseLenExpression()
	case lexer.TOKEN_CAP:
		left = p.parseCapExpression()
	case lexer.TOKEN_APPEND:
		left = p.parseAppendExpression()
	case lexer.TOKEN_COPY:
		left = p.parseCopyExpression()
	case lexer.TOKEN_DELETE:
		left = p.parseDeleteExpression()
	case lexer.TOKEN_STRUCT:
		left = p.parseStructType()
	case lexer.TOKEN_INTERFACE:
		left = p.parseInterfaceType()
	case lexer.TOKEN_CHAN:
		left = p.parseChanType()
	default:
		return nil
	}

	// 解析中缀表达式
	for !p.peekTokenIs(lexer.TOKEN_SEMICOLON) && precedence < p.peekPrecedence() {
		switch p.peekToken.Type {
		case lexer.TOKEN_PLUS, lexer.TOKEN_MINUS, lexer.TOKEN_ASTERISK, lexer.TOKEN_SLASH,
			lexer.TOKEN_PERCENT, lexer.TOKEN_EQ, lexer.TOKEN_NOT_EQ, lexer.TOKEN_LT,
			lexer.TOKEN_GT, lexer.TOKEN_LT_EQ, lexer.TOKEN_GT_EQ, lexer.TOKEN_AND,
			lexer.TOKEN_OR, lexer.TOKEN_BIT_AND, lexer.TOKEN_BIT_OR, lexer.TOKEN_BIT_XOR,
			lexer.TOKEN_SHL, lexer.TOKEN_SHR:
			p.nextToken()
			left = p.parseInfixExpression(left)
		case lexer.TOKEN_LPAREN:
			p.nextToken()
			left = p.parseCallExpression(left)
		case lexer.TOKEN_LBRACKET:
			p.nextToken()
			left = p.parseIndexExpression(left)
		case lexer.TOKEN_DOT:
			p.nextToken()
			left = p.parseSelectorExpression(left)
		case lexer.TOKEN_DOUBLE_COLON:
			p.nextToken()
			left = p.parseStaticAccessExpression(left)
		default:
			return left
		}
	}

	// 检查结构体字面量（只在表达式末尾）
	if p.peekTokenIs(lexer.TOKEN_LBRACE) {
		if ident, ok := left.(*Identifier); ok {
			// 检查是否是类型名（首字母大写或特定关键字）
			if len(ident.Value) > 0 && (ident.Value[0] >= 'A' && ident.Value[0] <= 'Z') {
				p.nextToken()
				left = p.parseStructLiteral(left)
			}
		}
	}

	return left
}

// parseGroupedExpression 解析括号表达式
func (p *Parser) parseGroupedExpression() Expression {
	token := p.curToken
	p.nextToken()
	expr := p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	// 检查是否是类型转换
	if _, ok := expr.(*Identifier); ok {
		return &ParenExpr{Token: token, X: expr}
	}

	return &ParenExpr{Token: token, X: expr}
}

// parsePrefixExpression 解析前缀表达式
func (p *Parser) parsePrefixExpression() Expression {
	expr := &UnaryExpr{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expr.Operand = p.parseExpression(PREFIX)
	return expr
}

// parseInfixExpression 解析中缀表达式
func (p *Parser) parseInfixExpression(left Expression) Expression {
	expr := &BinaryExpr{
		Token:    p.curToken,
		Left:     left,
		Operator: p.curToken.Literal,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expr.Right = p.parseExpression(precedence)
	return expr
}

// parseCallExpression 解析函数调用表达式
func (p *Parser) parseCallExpression(function Expression) Expression {
	expr := &CallExpr{Token: p.curToken, Function: function}
	expr.Arguments = p.parseCallArguments()
	return expr
}

// parseCallArguments 解析调用参数
func (p *Parser) parseCallArguments() []Expression {
	var args []Expression

	if p.peekTokenIs(lexer.TOKEN_RPAREN) {
		p.nextToken()
		return args
	}

	p.nextToken()
	args = append(args, p.parseExpression(LOWEST))

	for p.peekTokenIs(lexer.TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		args = append(args, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return args
}

// parseIndexExpression 解析索引表达式
func (p *Parser) parseIndexExpression(left Expression) Expression {
	token := p.curToken
	p.nextToken()

	// 检查是否是切片表达式
	if p.curTokenIs(lexer.TOKEN_COLON) {
		return p.parseSliceExpression(left, token, nil)
	}

	index := p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.TOKEN_COLON) {
		p.nextToken()
		return p.parseSliceExpression(left, token, index)
	}

	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}

	return &IndexExpr{Token: token, X: left, Index: index}
}

// parseSliceExpression 解析切片表达式
func (p *Parser) parseSliceExpression(x Expression, token lexer.Token, low Expression) Expression {
	expr := &SliceExpr{Token: token, X: x, Low: low}
	p.nextToken()

	if !p.curTokenIs(lexer.TOKEN_RBRACKET) && !p.curTokenIs(lexer.TOKEN_COLON) {
		expr.High = p.parseExpression(LOWEST)
	}

	if p.peekTokenIs(lexer.TOKEN_COLON) {
		p.nextToken()
		p.nextToken()
		expr.Max = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}

	return expr
}

// parseSelectorExpression 解析选择器表达式
func (p *Parser) parseSelectorExpression(left Expression) Expression {
	token := p.curToken
	p.nextToken()

	// 检查是否是类型断言
	if p.curTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		typ := p.parseType()
		if !p.expectPeek(lexer.TOKEN_RPAREN) {
			return nil
		}
		return &TypeAssertExpr{Token: token, X: left, Type: typ}
	}

	if !p.curTokenIs(lexer.TOKEN_IDENT) {
		return nil
	}

	return &SelectorExpr{Token: token, X: left, Sel: p.curToken.Literal}
}

// parseStaticAccessExpression 解析静态访问表达式 (ClassName::member 或 self::member)
func (p *Parser) parseStaticAccessExpression(left Expression) Expression {
	token := p.curToken
	p.nextToken()

	// 允许标识符或 class 关键字（用于 ClassName::class 获取类信息）
	if !p.curTokenIs(lexer.TOKEN_IDENT) && !p.curTokenIs(lexer.TOKEN_CLASS) {
		p.addError("expected member name after ::")
		return nil
	}

	return &StaticAccessExpr{Token: token, Left: left, Member: p.curToken.Literal}
}

// parseArrayOrSliceLiteral 解析数组或切片字面量
func (p *Parser) parseArrayOrSliceLiteral() Expression {
	token := p.curToken
	p.nextToken()

	// 检查是否是切片类型
	if p.curTokenIs(lexer.TOKEN_RBRACKET) {
		// 切片类型
		p.nextToken()
		elt := p.parseType()
		if p.peekTokenIs(lexer.TOKEN_LBRACE) {
			p.nextToken()
			return p.parseSliceLiteralBody(token, elt)
		}
		return &SliceType{Token: token, Elt: elt}
	}

	// 数组类型或字面量
	lenExpr := p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}
	p.nextToken()
	elt := p.parseType()

	if p.peekTokenIs(lexer.TOKEN_LBRACE) {
		p.nextToken()
		return p.parseArrayLiteralBody(token, lenExpr, elt)
	}

	return &ArrayType{Token: token, Len: lenExpr, Elt: elt}
}

// parseArrayLiteralBody 解析数组字面量体
func (p *Parser) parseArrayLiteralBody(token lexer.Token, lenExpr, elt Expression) Expression {
	lit := &ArrayLiteral{Token: token, Type: elt}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		lit.Elements = append(lit.Elements, p.parseExpression(LOWEST))
		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if !p.curTokenIs(lexer.TOKEN_RBRACE) {
		p.expectPeek(lexer.TOKEN_RBRACE)
	}

	return lit
}

// parseSliceLiteralBody 解析切片字面量体
func (p *Parser) parseSliceLiteralBody(token lexer.Token, elt Expression) Expression {
	lit := &SliceLiteral{Token: token, Type: elt}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		lit.Elements = append(lit.Elements, p.parseExpression(LOWEST))
		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if !p.curTokenIs(lexer.TOKEN_RBRACE) {
		p.expectPeek(lexer.TOKEN_RBRACE)
	}

	return lit
}

// parseMapLiteral 解析 map 字面量
func (p *Parser) parseMapLiteral() Expression {
	token := p.curToken

	if !p.expectPeek(lexer.TOKEN_LBRACKET) {
		return nil
	}
	p.nextToken()
	keyType := p.parseType()

	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}
	p.nextToken()
	valType := p.parseType()

	if !p.peekTokenIs(lexer.TOKEN_LBRACE) {
		return &MapType{Token: token, Key: keyType, Value: valType}
	}

	p.nextToken()
	lit := &MapLiteral{Token: token, KeyType: keyType, ValType: valType}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		key := p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.TOKEN_COLON) {
			return nil
		}
		p.nextToken()
		value := p.parseExpression(LOWEST)
		lit.Pairs = append(lit.Pairs, &KeyValuePair{Key: key, Value: value})

		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if !p.curTokenIs(lexer.TOKEN_RBRACE) {
		p.expectPeek(lexer.TOKEN_RBRACE)
	}

	return lit
}

// parseStructLiteral 解析结构体字面量
func (p *Parser) parseStructLiteral(typeExpr Expression) Expression {
	lit := &StructLiteral{Token: p.curToken, Type: typeExpr}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		var fv *FieldValue
		if p.curTokenIs(lexer.TOKEN_IDENT) && p.peekTokenIs(lexer.TOKEN_COLON) {
			// 命名字段
			name := p.curToken.Literal
			p.nextToken()
			p.nextToken()
			value := p.parseExpression(LOWEST)
			fv = &FieldValue{Name: name, Value: value}
		} else {
			// 未命名字段
			value := p.parseExpression(LOWEST)
			fv = &FieldValue{Value: value}
		}
		lit.Fields = append(lit.Fields, fv)

		if p.peekTokenIs(lexer.TOKEN_COMMA) {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if !p.curTokenIs(lexer.TOKEN_RBRACE) {
		p.expectPeek(lexer.TOKEN_RBRACE)
	}

	return lit
}

// parseFuncLiteral 解析函数字面量
func (p *Parser) parseFuncLiteral() Expression {
	lit := &FuncLiteral{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	lit.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 解析返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		lit.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if !p.peekTokenIs(lexer.TOKEN_LBRACE) {
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			lit.Results = []*Field{{Type: typ}}
		}
	}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	lit.Body = p.parseBlockStmt()

	return lit
}

// parseReceiveExpression 解析接收表达式
func (p *Parser) parseReceiveExpression() Expression {
	token := p.curToken
	p.nextToken()
	return &ReceiveExpr{Token: token, X: p.parseExpression(PREFIX)}
}

// parseMakeExpression 解析 make 表达式
func (p *Parser) parseMakeExpression() Expression {
	expr := &MakeExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Type = p.parseType()

	for p.peekTokenIs(lexer.TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		expr.Args = append(expr.Args, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseNewExpression 解析 new 表达式
// 支持两种语法:
// 1. Go 风格: new(Type)
// 2. OOP 风格: new ClassName() 或 new ClassName(arg1, arg2)
func (p *Parser) parseNewExpression() Expression {
	expr := &NewExpr{Token: p.curToken}

	// 检查下一个 token
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		// Go 风格: new(Type)
		p.nextToken() // 消费 (
		p.nextToken()
		expr.Type = p.parseType()

		if !p.expectPeek(lexer.TOKEN_RPAREN) {
			return nil
		}
	} else if p.peekTokenIs(lexer.TOKEN_IDENT) {
		// OOP 风格: new ClassName(args)
		p.nextToken() // 消费 ClassName
		expr.Type = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		// 检查是否有参数列表
		if p.peekTokenIs(lexer.TOKEN_LPAREN) {
			p.nextToken() // 消费 (
			expr.Arguments = p.parseCallArguments()
		}
	} else {
		p.addError("expected type or class name after 'new'")
		return nil
	}

	return expr
}

// parseLenExpression 解析 len 表达式
func (p *Parser) parseLenExpression() Expression {
	expr := &LenExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.X = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseCapExpression 解析 cap 表达式
func (p *Parser) parseCapExpression() Expression {
	expr := &CapExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.X = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseAppendExpression 解析 append 表达式
func (p *Parser) parseAppendExpression() Expression {
	expr := &AppendExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Slice = p.parseExpression(LOWEST)

	for p.peekTokenIs(lexer.TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		expr.Elems = append(expr.Elems, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseCopyExpression 解析 copy 表达式
func (p *Parser) parseCopyExpression() Expression {
	expr := &CopyExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Dst = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_COMMA) {
		return nil
	}
	p.nextToken()
	expr.Src = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseDeleteExpression 解析 delete 表达式
func (p *Parser) parseDeleteExpression() Expression {
	expr := &DeleteExpr{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Map = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_COMMA) {
		return nil
	}
	p.nextToken()
	expr.Key = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.TOKEN_RPAREN) {
		return nil
	}

	return expr
}

// parseStructType 解析结构体类型
func (p *Parser) parseStructType() Expression {
	st := &StructType{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		field := p.parseStructField()
		if field != nil {
			st.Fields = append(st.Fields, field)
		}
		p.nextToken()
	}

	return st
}

// parseInterfaceType 解析接口类型
func (p *Parser) parseInterfaceType() Expression {
	it := &InterfaceType{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()

	for !p.curTokenIs(lexer.TOKEN_RBRACE) && !p.curTokenIs(lexer.TOKEN_EOF) {
		sig := p.parseFuncSignature()
		if sig != nil {
			it.Methods = append(it.Methods, sig)
		}
		p.nextToken()
	}

	return it
}

// parseChanType 解析通道类型
func (p *Parser) parseChanType() Expression {
	ct := &ChanType{Token: p.curToken, Dir: 0}
	p.nextToken()

	// 检查是否是 chan<- 或 <-chan
	if p.curTokenIs(lexer.TOKEN_ARROW) {
		ct.Dir = 1 // 只发送
		p.nextToken()
	}

	ct.Value = p.parseType()
	return ct
}

// parseType 解析类型
func (p *Parser) parseType() Expression {
	switch p.curToken.Type {
	case lexer.TOKEN_IDENT:
		return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.TOKEN_ASTERISK:
		token := p.curToken
		p.nextToken()
		return &PointerType{Token: token, Base: p.parseType()}
	case lexer.TOKEN_LBRACKET:
		return p.parseArrayOrSliceType()
	case lexer.TOKEN_MAP:
		return p.parseMapType()
	case lexer.TOKEN_CHAN:
		return p.parseChanType()
	case lexer.TOKEN_ARROW:
		// <-chan
		token := p.curToken
		p.nextToken()
		if p.curTokenIs(lexer.TOKEN_CHAN) {
			p.nextToken()
			return &ChanType{Token: token, Dir: 2, Value: p.parseType()}
		}
		return nil
	case lexer.TOKEN_FUNC:
		return p.parseFuncType()
	case lexer.TOKEN_STRUCT:
		return p.parseStructType()
	case lexer.TOKEN_INTERFACE:
		return p.parseInterfaceType()
	case lexer.TOKEN_ELLIPSIS:
		token := p.curToken
		p.nextToken()
		return &Ellipsis{Token: token, Elt: p.parseType()}
	default:
		return nil
	}
}

// parseArrayOrSliceType 解析数组或切片类型
func (p *Parser) parseArrayOrSliceType() Expression {
	token := p.curToken
	p.nextToken()

	if p.curTokenIs(lexer.TOKEN_RBRACKET) {
		// 切片类型
		p.nextToken()
		return &SliceType{Token: token, Elt: p.parseType()}
	}

	// 数组类型
	lenExpr := p.parseExpression(LOWEST)
	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}
	p.nextToken()
	return &ArrayType{Token: token, Len: lenExpr, Elt: p.parseType()}
}

// parseMapType 解析 map 类型
func (p *Parser) parseMapType() Expression {
	token := p.curToken

	if !p.expectPeek(lexer.TOKEN_LBRACKET) {
		return nil
	}
	p.nextToken()
	keyType := p.parseType()

	if !p.expectPeek(lexer.TOKEN_RBRACKET) {
		return nil
	}
	p.nextToken()
	valType := p.parseType()

	return &MapType{Token: token, Key: keyType, Value: valType}
}

// parseFuncType 解析函数类型
func (p *Parser) parseFuncType() Expression {
	ft := &FuncType{Token: p.curToken}

	if !p.expectPeek(lexer.TOKEN_LPAREN) {
		return nil
	}
	ft.Params = p.parseFieldList(lexer.TOKEN_RPAREN)

	// 解析返回值
	if p.peekTokenIs(lexer.TOKEN_LPAREN) {
		p.nextToken()
		ft.Results = p.parseFieldList(lexer.TOKEN_RPAREN)
	} else if !p.peekTokenIs(lexer.TOKEN_EOF) && !p.peekTokenIs(lexer.TOKEN_SEMICOLON) && !p.peekTokenIs(lexer.TOKEN_COMMA) && !p.peekTokenIs(lexer.TOKEN_RPAREN) && !p.peekTokenIs(lexer.TOKEN_RBRACE) {
		p.nextToken()
		typ := p.parseType()
		if typ != nil {
			ft.Results = []*Field{{Type: typ}}
		}
	}

	return ft
}

// Parse 解析源代码
func Parse(input string) (*File, []string) {
	l := lexer.New(input)
	p := New(l)
	file := p.ParseFile()
	return file, p.Errors()
}
