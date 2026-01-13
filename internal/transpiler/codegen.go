package transpiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tangzhangming/tugo/internal/i18n"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
)

// CodeGen 代码生成器
type CodeGen struct {
	transpiler         *Transpiler
	builder            strings.Builder
	indent             int
	userImports        []*parser.ImportSpec // 用户定义的导入
	currentReceiver    string               // 当前类方法的接收者（用于翻译 this）
	currentStaticClass *parser.ClassDecl    // 当前静态类（用于翻译 self::）
	currentStructDecl  *parser.StructDecl   // 当前结构体（用于字段名翻译）
	currentClassDecl   *parser.ClassDecl    // 当前类（用于字段名翻译）
	typeToPackage      map[string]string    // 类型名到包名的映射 (User -> models)
	goImports          map[string]bool      // Go 标准库导入
	tugoImports        map[string]string    // tugo 包导入 (合并后) pkgPath -> pkgName
	currentFuncErrable bool                 // 当前函数是否是 errable
	currentFuncResults []*parser.Field      // 当前函数的返回值类型
	tryCounter         int                  // try 块计数器（用于生成唯一标签）
	inTryBlock         bool                 // 是否在 try 块内
	methodOverloads    map[string]bool      // 当前类/结构体的重载方法名 (key: methodName)
	varTypes           map[string]string    // 变量名到类型名的映射（用于重载解析）
}

// NewCodeGen 创建一个新的代码生成器
func NewCodeGen(t *Transpiler) *CodeGen {
	return &CodeGen{
		transpiler:      t,
		typeToPackage:   make(map[string]string),
		goImports:       make(map[string]bool),
		tugoImports:     make(map[string]string),
		methodOverloads: make(map[string]bool),
		varTypes:        make(map[string]string),
	}
}

// Generate 生成 Go 代码
func (g *CodeGen) Generate(file *parser.File) string {
	g.builder.Reset()

	// 重置状态
	g.typeToPackage = make(map[string]string)
	g.goImports = make(map[string]bool)
	g.tugoImports = make(map[string]string)

	// 收集并处理用户导入
	for _, imp := range file.Imports {
		for _, spec := range imp.Specs {
			g.userImports = append(g.userImports, spec)
			g.processImportSpec(spec)
		}
	}

	// 预扫描以确定需要哪些导入
	g.prescan(file)

	// 生成 package 声明
	g.writeLine("package " + file.Package)
	g.writeLine("")

	// 生成 import 声明
	g.generateImports()

	// 生成语句
	for _, stmt := range file.Statements {
		g.generateStatement(stmt)
		g.writeLine("")
	}

	return g.builder.String()
}

// processImportSpec 处理单个导入项
func (g *CodeGen) processImportSpec(spec *parser.ImportSpec) {
	if spec.FromGo {
		// Go 标准库导入
		g.goImports[spec.Path] = true
	} else if spec.TypeName != "" {
		// tugo 风格导入 (com.company.demo.models.User)
		// 记录类型名到包名的映射
		typeName := spec.TypeName
		if spec.Alias != "" {
			typeName = spec.Alias
		}
		g.typeToPackage[typeName] = spec.PkgName

		// 转换包路径并合并
		// com.company.demo.models -> com.company.demo/models
		goPkgPath := g.convertPkgPath(spec.PkgPath)
		g.tugoImports[goPkgPath] = spec.PkgName
	}
}

// convertPkgPath 将 tugo 包路径转换为 Go import 路径
// com.company.demo.models -> com.company.demo/models
func (g *CodeGen) convertPkgPath(pkgPath string) string {
	// 获取项目模块名
	cfg := g.transpiler.GetConfig()
	module := cfg.Project.Module

	// 如果路径以模块名开头，则转换
	if len(pkgPath) >= len(module) && pkgPath[:len(module)] == module {
		// 模块名之后的部分用斜杠分隔
		suffix := pkgPath[len(module):]
		if suffix == "" {
			return module
		}
		// 去掉开头的点，然后把点替换为斜杠
		if suffix[0] == '.' {
			suffix = suffix[1:]
		}
		suffix = strings.ReplaceAll(suffix, ".", "/")
		return module + "/" + suffix
	}

	// tugo 标准库路径处理
	if len(pkgPath) >= 5 && pkgPath[:5] == "tugo." {
		// tugo.lang -> tugo/lang
		return strings.ReplaceAll(pkgPath, ".", "/")
	}

	// 其他情况直接替换点为斜杠
	return strings.ReplaceAll(pkgPath, ".", "/")
}

// prescan 预扫描文件以确定需要哪些导入
func (g *CodeGen) prescan(file *parser.File) {
	for _, stmt := range file.Statements {
		g.prescanStatement(stmt)

		// 扫描类的方法，并添加 tugo/lang 导入（用于 ClassInfo）
		if classDecl, ok := stmt.(*parser.ClassDecl); ok {
			// 非静态类需要 ClassInfo，添加 tugo/lang 导入
			if !classDecl.Static {
				g.tugoImports["tugo/lang"] = "lang"
			}
			for _, method := range classDecl.Methods {
				if method.Body != nil {
					g.prescanBlock(method.Body)
				}
			}
		}

		// 扫描结构体的方法
		if structDecl, ok := stmt.(*parser.StructDecl); ok {
			for _, method := range structDecl.Methods {
				if method.Body != nil {
					g.prescanBlock(method.Body)
				}
			}
		}
	}
}

// prescanStatement 预扫描语句
func (g *CodeGen) prescanStatement(stmt parser.Statement) {
	switch s := stmt.(type) {
	case *parser.FuncDecl:
		if s.Body != nil {
			g.prescanBlock(s.Body)
		}
	case *parser.ExpressionStmt:
		g.prescanExpression(s.Expression)
	case *parser.BlockStmt:
		g.prescanBlock(s)
	case *parser.IfStmt:
		g.prescanExpression(s.Condition)
		if s.Consequence != nil {
			g.prescanBlock(s.Consequence)
		}
		if s.Alternative != nil {
			g.prescanStatement(s.Alternative)
		}
	case *parser.ForStmt:
		if s.Body != nil {
			g.prescanBlock(s.Body)
		}
	case *parser.RangeStmt:
		if s.Body != nil {
			g.prescanBlock(s.Body)
		}
	case *parser.SwitchStmt:
		for _, c := range s.Cases {
			for _, stmt := range c.Body {
				g.prescanStatement(stmt)
			}
		}
	case *parser.ReturnStmt:
		for _, v := range s.Values {
			g.prescanExpression(v)
		}
	case *parser.VarDecl:
		if s.Value != nil {
			g.prescanExpression(s.Value)
		}
	case *parser.ShortVarDecl:
		if s.Value != nil {
			g.prescanExpression(s.Value)
		}
	case *parser.AssignStmt:
		for _, r := range s.Right {
			g.prescanExpression(r)
		}
	case *parser.ThrowStmt:
		if s.Value != nil {
			g.prescanExpression(s.Value)
		}
	case *parser.TryStmt:
		if s.Body != nil {
			g.prescanBlock(s.Body)
		}
		if s.Catch != nil && s.Catch.Body != nil {
			g.prescanBlock(s.Catch.Body)
		}
	}
}

// prescanBlock 预扫描代码块
func (g *CodeGen) prescanBlock(block *parser.BlockStmt) {
	for _, stmt := range block.Statements {
		g.prescanStatement(stmt)
	}
}

// prescanExpression 预扫描表达式
func (g *CodeGen) prescanExpression(expr parser.Expression) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查是否是全局函数调用
		if ident, ok := e.Function.(*parser.Identifier); ok {
			switch ident.Value {
			case "print", "println", "print_f", "errorf":
				g.transpiler.SetNeedFmt(true)
			}
		}
		for _, arg := range e.Arguments {
			g.prescanExpression(arg)
		}
	case *parser.BinaryExpr:
		g.prescanExpression(e.Left)
		g.prescanExpression(e.Right)
	case *parser.UnaryExpr:
		g.prescanExpression(e.Operand)
	case *parser.IndexExpr:
		g.prescanExpression(e.X)
		g.prescanExpression(e.Index)
	case *parser.SelectorExpr:
		g.prescanExpression(e.X)
	case *parser.SliceExpr:
		g.prescanExpression(e.X)
		g.prescanExpression(e.Low)
		g.prescanExpression(e.High)
		g.prescanExpression(e.Max)
	}
}

// generateImports 生成导入声明
func (g *CodeGen) generateImports() {
	// 收集所有需要的导入
	imports := make(map[string]bool) // path -> exists

	// Go 标准库导入
	for path := range g.goImports {
		imports[path] = true
	}

	// tugo 包导入（已合并）
	for path := range g.tugoImports {
		imports[path] = true
	}

	// 自动导入 fmt
	if g.transpiler.NeedFmt() {
		imports["fmt"] = true
	}

	if len(imports) == 0 {
		return
	}

	// 排序导入
	var paths []string
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	if len(paths) == 1 {
		g.writeLine(fmt.Sprintf("import \"%s\"", paths[0]))
	} else {
		g.writeLine("import (")
		g.indent++
		for _, path := range paths {
			g.writeLine(fmt.Sprintf("\"%s\"", path))
		}
		g.indent--
		g.writeLine(")")
	}
	g.writeLine("")
}

// generateStatement 生成语句
func (g *CodeGen) generateStatement(stmt parser.Statement) {
	switch s := stmt.(type) {
	case *parser.FuncDecl:
		g.generateFuncDecl(s)
	case *parser.StructDecl:
		g.generateStructDecl(s)
	case *parser.ClassDecl:
		g.generateClassDecl(s)
	case *parser.InterfaceDecl:
		g.generateInterfaceDecl(s)
	case *parser.TypeDecl:
		g.generateTypeDecl(s)
	case *parser.VarDecl:
		g.generateVarDecl(s)
	case *parser.ConstDecl:
		g.generateConstDecl(s)
	case *parser.ShortVarDecl:
		g.generateShortVarDecl(s)
	case *parser.AssignStmt:
		g.generateAssignStmt(s)
	case *parser.ReturnStmt:
		g.generateReturnStmt(s)
	case *parser.ThrowStmt:
		g.generateThrowStmt(s)
	case *parser.TryStmt:
		g.generateTryStmt(s)
	case *parser.IfStmt:
		g.generateIfStmt(s)
	case *parser.ForStmt:
		g.generateForStmt(s)
	case *parser.RangeStmt:
		g.generateRangeStmt(s)
	case *parser.SwitchStmt:
		g.generateSwitchStmt(s)
	case *parser.SelectStmt:
		g.generateSelectStmt(s)
	case *parser.GoStmt:
		g.generateGoStmt(s)
	case *parser.DeferStmt:
		g.generateDeferStmt(s)
	case *parser.BreakStmt:
		g.generateBreakStmt(s)
	case *parser.ContinueStmt:
		g.generateContinueStmt(s)
	case *parser.FallthroughStmt:
		g.writeLine("fallthrough")
	case *parser.BlockStmt:
		g.generateBlockStmt(s)
	case *parser.ExpressionStmt:
		// 如果在 errable 函数中且表达式是 errable 调用，自动添加错误检查
		if g.currentFuncErrable && g.isErrableCall(s.Expression) {
			g.generateErrableCallStmt(s.Expression)
		} else {
			g.writeLine(g.generateExpression(s.Expression))
		}
	case *parser.SendStmt:
		g.generateSendStmt(s)
	case *parser.IncDecStmt:
		g.generateIncDecStmt(s)
	}
}

// generateFuncDecl 生成函数声明
func (g *CodeGen) generateFuncDecl(decl *parser.FuncDecl) {
	// 检查是否有默认参数
	hasDefault := false
	for _, param := range decl.Params {
		if param.DefaultValue != nil {
			hasDefault = true
			break
		}
	}

	if hasDefault {
		g.generateFuncWithDefaults(decl)
		return
	}

	// 生成普通函数
	g.write("func ")

	// 接收者
	if decl.Receiver != nil {
		g.write("(")
		if decl.Receiver.Name != "" {
			g.write(decl.Receiver.Name + " ")
		}
		g.write(g.generateType(decl.Receiver.Type))
		g.write(") ")
	}

	// 函数名
	name := symbol.ToGoName(decl.Name, decl.Public)
	g.write(name)

	// 参数
	g.write("(")
	g.generateParams(decl.Params)
	g.write(")")

	// 返回值
	if len(decl.Results) > 0 {
		g.write(" ")
		if decl.Errable {
			// errable 函数：追加 error 返回值
			g.write("(")
			g.generateParams(decl.Results)
			g.write(", error)")
		} else if len(decl.Results) == 1 && decl.Results[0].Name == "" {
			g.write(g.generateType(decl.Results[0].Type))
		} else {
			g.write("(")
			g.generateParams(decl.Results)
			g.write(")")
		}
	} else if decl.Errable {
		// 无返回值但是 errable：只返回 error
		g.write(" error")
	}

	// 函数体
	if decl.Body != nil {
		g.write(" ")
		// 设置当前函数是否是 errable 及返回类型
		g.currentFuncErrable = decl.Errable
		g.currentFuncResults = decl.Results
		g.generateBlockStmtInline(decl.Body)
		g.currentFuncErrable = false
		g.currentFuncResults = nil
	}
	g.writeLine("")
}

// generateFuncWithDefaults 生成带默认参数的函数
func (g *CodeGen) generateFuncWithDefaults(decl *parser.FuncDecl) {
	funcName := symbol.ToGoName(decl.Name, decl.Public)
	optsName := funcName + "__Opts"

	// 生成 Options 结构体
	g.writeLine(fmt.Sprintf("type %s struct {", optsName))
	g.indent++
	for _, param := range decl.Params {
		fieldName := symbol.ToGoName(param.Name, true)
		typeName := g.generateType(param.Type)
		g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成默认值构造器
	g.writeLine(fmt.Sprintf("func NewDefault__%s() %s {", optsName, optsName))
	g.indent++
	var defaultFields []string
	for _, param := range decl.Params {
		if param.DefaultValue != nil {
			fieldName := symbol.ToGoName(param.Name, true)
			defaultVal := g.generateExpression(param.DefaultValue)
			defaultFields = append(defaultFields, fmt.Sprintf("%s: %s", fieldName, defaultVal))
		}
	}
	g.writeLine(fmt.Sprintf("return %s{%s}", optsName, strings.Join(defaultFields, ", ")))
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成函数
	g.writeIndent()
	g.write("func ")

	// 接收者
	if decl.Receiver != nil {
		g.write("(")
		if decl.Receiver.Name != "" {
			g.write(decl.Receiver.Name + " ")
		}
		g.write(g.generateType(decl.Receiver.Type))
		g.write(") ")
	}

	g.write(funcName)
	g.write(fmt.Sprintf("(opts %s)", optsName))

	// 返回值
	if len(decl.Results) > 0 {
		g.write(" ")
		if len(decl.Results) == 1 && decl.Results[0].Name == "" {
			g.write(g.generateType(decl.Results[0].Type))
		} else {
			g.write("(")
			g.generateParams(decl.Results)
			g.write(")")
		}
	}

	g.writeLine(" {")
	g.indent++

	// 直接从 opts 获取参数值
	for _, param := range decl.Params {
		varName := symbol.TransformDollarVar(param.Name)
		fieldName := symbol.ToGoName(param.Name, true)
		g.writeLine(fmt.Sprintf("%s := opts.%s", varName, fieldName))
	}

	// 生成函数体
	if decl.Body != nil {
		for _, stmt := range decl.Body.Statements {
			g.generateStatement(stmt)
		}
	}

	g.indent--
	g.writeLine("}")
}

// generateParams 生成参数列表
func (g *CodeGen) generateParams(params []*parser.Field) {
	for i, param := range params {
		if i > 0 {
			g.write(", ")
		}
		if param.Name != "" {
			g.write(symbol.TransformDollarVar(param.Name))
			g.write(" ")
		}
		g.write(g.generateType(param.Type))
	}
}

// generateStructDecl 生成结构体声明
func (g *CodeGen) generateStructDecl(decl *parser.StructDecl) {
	structName := symbol.ToGoName(decl.Name, decl.Public)

	// 1. 生成结构体定义
	g.writeLine(fmt.Sprintf("type %s struct {", structName))
	g.indent++

	// 嵌入字段
	for _, embed := range decl.Embeds {
		g.writeLine(embed)
	}

	// 普通字段
	for _, field := range decl.Fields {
		fieldName := symbol.ToGoName(field.Name, field.Public)
		typeName := g.generateType(field.Type)
		if field.Tag != "" {
			g.writeLine(fmt.Sprintf("%s %s %s", fieldName, typeName, field.Tag))
		} else {
			g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
		}
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 2. 生成构造函数（如果有 init）
	if decl.InitMethod != nil {
		g.generateStructConstructor(decl, structName)
		g.writeLine("")
	}

	// 3. 生成方法
	for _, method := range decl.Methods {
		g.generateStructMethod(decl, structName, method)
		g.writeLine("")
	}
}

// generateStructConstructor 生成结构体构造函数
func (g *CodeGen) generateStructConstructor(decl *parser.StructDecl, structName string) {
	init := decl.InitMethod

	// 检查是否有默认参数
	hasDefaults := false
	for _, param := range init.Params {
		if param.DefaultValue != nil {
			hasDefaults = true
			break
		}
	}

	if hasDefaults {
		// 生成 Opts 结构体
		optsName := structName + "__InitOpts"
		g.writeLine(fmt.Sprintf("type %s struct {", optsName))
		g.indent++
		for _, param := range init.Params {
			paramName := symbol.ToGoName(param.Name, true)
			typeName := g.generateType(param.Type)
			g.writeLine(fmt.Sprintf("%s %s", paramName, typeName))
		}
		g.indent--
		g.writeLine("}")
		g.writeLine("")

		// 生成 NewDefaultOpts 函数
		g.writeLine(fmt.Sprintf("func NewDefault__%s() %s {", optsName, optsName))
		g.indent++
		g.write(fmt.Sprintf("return %s{", optsName))
		first := true
		for _, param := range init.Params {
			if param.DefaultValue != nil {
				if !first {
					g.write(", ")
				}
				paramName := symbol.ToGoName(param.Name, true)
				g.write(fmt.Sprintf("%s: %s", paramName, g.generateExpression(param.DefaultValue)))
				first = false
			}
		}
		g.writeLine("}")
		g.indent--
		g.writeLine("}")
		g.writeLine("")

		// 生成构造函数（使用 Opts）
		g.writeLine(fmt.Sprintf("func New__%s(opts %s) *%s {", structName, optsName, structName))
		g.indent++
		g.writeLine(fmt.Sprintf("s := &%s{}", structName))

		// 提取参数
		for _, param := range init.Params {
			paramName := param.Name
			goParamName := symbol.ToGoName(param.Name, true)
			g.writeLine(fmt.Sprintf("%s := opts.%s", paramName, goParamName))
		}

		// 生成 init 方法体（翻译 this 为 s）
		g.currentReceiver = "s"
		g.currentStructDecl = decl
		if init.Body != nil {
			for _, stmt := range init.Body.Statements {
				g.generateStatement(stmt)
			}
		}
		g.currentReceiver = ""
		g.currentStructDecl = nil

		g.writeLine("return s")
		g.indent--
		g.writeLine("}")
	} else {
		// 没有默认参数，直接生成构造函数
		g.write(fmt.Sprintf("func New__%s(", structName))
		g.generateParams(init.Params)
		g.writeLine(fmt.Sprintf(") *%s {", structName))
		g.indent++
		g.writeLine(fmt.Sprintf("s := &%s{}", structName))

		// 生成 init 方法体
		g.currentReceiver = "s"
		g.currentStructDecl = decl
		if init.Body != nil {
			for _, stmt := range init.Body.Statements {
				g.generateStatement(stmt)
			}
		}
		g.currentReceiver = ""
		g.currentStructDecl = nil

		g.writeLine("return s")
		g.indent--
		g.writeLine("}")
	}
}

// generateStructMethod 生成结构体方法（指针接收者）
func (g *CodeGen) generateStructMethod(decl *parser.StructDecl, structName string, method *parser.ClassMethod) {
	isPublic := method.Visibility == "public"
	
	// 使用原始结构体名进行重载查找
	originalName := decl.Name
	
	// 检查是否是重载方法，使用修饰后的名称
	var methodName string
	if g.transpiler.table.IsMethodOverloaded(g.transpiler.pkg, originalName, method.Name) {
		methodName = symbol.GenerateMangledName(method.Name, method.Params, isPublic)
	} else {
		methodName = symbol.ToGoName(method.Name, isPublic)
	}

	// 方法签名（指针接收者）
	g.write(fmt.Sprintf("func (s *%s) %s(", structName, methodName))
	g.generateParams(method.Params)
	g.write(")")

	// 返回值
	if len(method.Results) > 0 {
		g.write(" ")
		if method.Errable {
			// errable 方法：追加 error 返回值
			g.write("(")
			g.generateParams(method.Results)
			g.write(", error)")
		} else if len(method.Results) == 1 && method.Results[0].Name == "" {
			g.write(g.generateType(method.Results[0].Type))
		} else {
			g.write("(")
			g.generateParams(method.Results)
			g.write(")")
		}
	} else if method.Errable {
		// 无返回值但是 errable：只返回 error
		g.write(" error")
	}

	g.write(" ")

	// 方法体（翻译 this 为 s）
	g.currentReceiver = "s"
	g.currentStructDecl = decl
	g.currentFuncErrable = method.Errable
	g.currentFuncResults = method.Results
	g.generateBlockStmt(method.Body)
	g.currentReceiver = ""
	g.currentStructDecl = nil
	g.currentFuncErrable = false
	g.currentFuncResults = nil
}

// generateClassDecl 生成类声明
func (g *CodeGen) generateClassDecl(decl *parser.ClassDecl) {
	className := symbol.ToGoName(decl.Name, decl.Public)

	if decl.Static {
		// 静态类：生成包级变量和函数
		g.generateStaticClass(decl, className)
		return
	}

	if decl.Abstract {
		// 抽象类：生成接口 + 基础结构体
		g.generateAbstractClass(decl, className)
		return
	}

	if decl.Extends != "" {
		// 子类：嵌入父类基础结构体
		g.generateChildClass(decl, className)
		return
	}

	// 普通类
	g.generateNormalClass(decl, className)
}

// generateNormalClass 生成普通类
// generateStaticClass 生成静态类（包级变量和函数）
func (g *CodeGen) generateStaticClass(decl *parser.ClassDecl, className string) {
	// 记录当前静态类，用于 self:: 翻译
	g.currentReceiver = className
	g.currentStaticClass = decl

	// 1. 生成静态字段（包级变量）
	for _, field := range decl.Fields {
		isPublic := field.Visibility == "public"
		// 命名规则: 公开字段 -> ClassName + FieldName, 私有字段 -> _className_fieldName
		var varName string
		if isPublic {
			varName = className + symbol.ToGoName(field.Name, true)
		} else {
			varName = "_" + strings.ToLower(decl.Name) + "_" + field.Name
		}
		typeName := g.generateType(field.Type)
		if field.Value != nil {
			g.writeLine(fmt.Sprintf("var %s %s = %s", varName, typeName, g.generateExpression(field.Value)))
		} else {
			g.writeLine(fmt.Sprintf("var %s %s", varName, typeName))
		}
	}
	if len(decl.Fields) > 0 {
		g.writeLine("")
	}

	// 2. 生成静态方法（包级函数）
	for _, method := range decl.Methods {
		g.generateStaticClassMethod(decl, className, method)
		g.writeLine("")
	}

	g.currentReceiver = ""
	g.currentStaticClass = nil
}

// generateStaticClassMethod 生成静态类方法（包级函数）
func (g *CodeGen) generateStaticClassMethod(decl *parser.ClassDecl, className string, method *parser.ClassMethod) {
	isPublic := method.Visibility == "public"
	
	// 检查是否是重载方法
	isOverloaded := g.transpiler.table.IsMethodOverloaded(g.transpiler.pkg, decl.Name, method.Name)
	
	// 命名规则: 公开方法 -> ClassName + MethodName, 私有方法 -> className + MethodName (首字母小写)
	var funcName string
	if isOverloaded {
		// 重载方法使用修饰名
		mangledName := symbol.GenerateMangledName(method.Name, method.Params, isPublic)
		if isPublic {
			funcName = className + mangledName
		} else {
			funcName = strings.ToLower(string(decl.Name[0])) + decl.Name[1:] + mangledName
		}
	} else {
		if isPublic {
			funcName = className + symbol.ToGoName(method.Name, true)
		} else {
			funcName = strings.ToLower(string(decl.Name[0])) + decl.Name[1:] + symbol.ToGoName(method.Name, true)
		}
	}

	g.write(fmt.Sprintf("func %s(", funcName))
	g.generateParams(method.Params)
	g.write(")")

	// 返回值
	if len(method.Results) > 0 {
		g.write(" ")
		if len(method.Results) == 1 && method.Results[0].Name == "" {
			g.write(g.generateType(method.Results[0].Type))
		} else {
			g.write("(")
			g.generateParams(method.Results)
			g.write(")")
		}
	}

	g.write(" ")
	g.generateBlockStmt(method.Body)
}

func (g *CodeGen) generateNormalClass(decl *parser.ClassDecl, className string) {
	// 设置当前类上下文（用于字段名翻译）
	g.currentClassDecl = decl

	// 检测是否是入口类
	isEntryClass := g.transpiler.IsEntryClass(decl)
	var mainMethod *parser.ClassMethod
	if isEntryClass {
		// 查找 main 方法
		for _, method := range decl.Methods {
			if method.Name == "main" && method.Static {
				mainMethod = method
				break
			}
		}
	}

	// 1. 生成静态字段（包级变量）
	for _, field := range decl.Fields {
		if field.Static {
			varName := fmt.Sprintf("_%s_%s", className, field.Name)
			typeName := g.generateType(field.Type)
			if field.Value != nil {
				g.writeLine(fmt.Sprintf("var %s %s = %s", varName, typeName, g.generateExpression(field.Value)))
			} else {
				g.writeLine(fmt.Sprintf("var %s %s", varName, typeName))
			}
		}
	}

	// 1.5 生成类信息变量 _ClassName_classInfo
	g.generateClassInfo(decl, className)
	g.writeLine("")

	// 2. 生成结构体
	g.writeLine(fmt.Sprintf("type %s struct {", className))
	g.indent++
	for _, field := range decl.Fields {
		if !field.Static {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			typeName := g.generateType(field.Type)
			g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
		}
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 3. 生成构造函数
	if decl.InitMethod != nil {
		g.generateClassConstructor(decl, className)
	} else {
		// 生成默认构造函数
		g.generateDefaultConstructor(decl, className)
	}
	g.writeLine("")

	// 4. 生成方法
	for _, method := range decl.Methods {
		// 入口类的 main 方法单独处理
		if isEntryClass && method == mainMethod {
			continue
		}
		g.generateClassMethod(decl, className, method)
		g.writeLine("")
	}

	// 4.5 生成 Class() 方法
	g.generateClassMethod_Class(className)
	g.writeLine("")

	// 5. 入口类：生成 Go 的 func main()
	if isEntryClass && mainMethod != nil {
		g.generateGoMainFunc(decl, mainMethod)
		g.writeLine("")
	}

	// 清理当前类上下文
	g.currentClassDecl = nil
}

// generateClassInfo 生成类信息变量 _ClassName_classInfo
func (g *CodeGen) generateClassInfo(decl *parser.ClassDecl, className string) {
	pkgName := g.transpiler.pkg
	fullName := pkgName + "." + decl.Name

	// 需要导入 tugo/lang 包
	g.tugoImports["tugo/lang"] = "lang"

	// 生成类信息变量
	infoVarName := fmt.Sprintf("_%s_classInfo", className)

	// 检查是否有父类
	if decl.Extends != "" {
		parentClassName := symbol.ToGoName(decl.Extends, true)
		parentInfoVar := fmt.Sprintf("_%s_classInfo", parentClassName)
		// 如果父类在其他包，需要加包前缀
		if pkg, ok := g.typeToPackage[decl.Extends]; ok {
			parentInfoVar = pkg + "." + parentInfoVar
		}
		g.writeLine(fmt.Sprintf("var %s = &lang.ClassInfo{Name: %q, Package: %q, FullName: %q, Parent: %s}",
			infoVarName, decl.Name, pkgName, fullName, parentInfoVar))
	} else {
		g.writeLine(fmt.Sprintf("var %s = &lang.ClassInfo{Name: %q, Package: %q, FullName: %q}",
			infoVarName, decl.Name, pkgName, fullName))
	}
}

// generateClassMethod_Class 生成 Class() 方法
func (g *CodeGen) generateClassMethod_Class(className string) {
	infoVarName := fmt.Sprintf("_%s_classInfo", className)
	g.writeLine(fmt.Sprintf("func (this *%s) Class() *lang.ClassInfo {", className))
	g.indent++
	g.writeLine(fmt.Sprintf("return %s", infoVarName))
	g.indent--
	g.writeLine("}")
}

// generateAbstractClass 生成抽象类（接口 + 基础结构体）
func (g *CodeGen) generateAbstractClass(decl *parser.ClassDecl, className string) {
	baseName := symbol.ToGoName(decl.Name, false) + "Base"

	// 1. 生成静态字段（包级变量）
	for _, field := range decl.Fields {
		if field.Static {
			varName := fmt.Sprintf("_%s_%s", className, field.Name)
			typeName := g.generateType(field.Type)
			if field.Value != nil {
				g.writeLine(fmt.Sprintf("var %s %s = %s", varName, typeName, g.generateExpression(field.Value)))
			} else {
				g.writeLine(fmt.Sprintf("var %s %s", varName, typeName))
			}
		}
	}

	// 2. 生成接口（包含抽象方法）
	g.writeLine(fmt.Sprintf("type %s interface {", className))
	g.indent++
	for _, method := range decl.AbstractMethods {
		isPublic := method.Visibility == "public" || method.Visibility == "protected"
		methodName := symbol.ToGoName(method.Name, isPublic)
		g.write(methodName + "(")
		g.generateParams(method.Params)
		g.write(")")
		if len(method.Results) > 0 {
			g.write(" ")
			if len(method.Results) == 1 && method.Results[0].Name == "" {
				g.write(g.generateType(method.Results[0].Type))
			} else {
				g.write("(")
				g.generateParams(method.Results)
				g.write(")")
			}
		}
		g.writeLine("")
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 3. 生成基础结构体
	g.writeLine(fmt.Sprintf("type %s struct {", baseName))
	g.indent++
	for _, field := range decl.Fields {
		if !field.Static {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			typeName := g.generateType(field.Type)
			g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
		}
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 4. 生成基础结构体的具体方法
	for _, method := range decl.Methods {
		g.generateClassMethod(decl, baseName, method)
		g.writeLine("")
	}
}

// generateChildClass 生成子类（嵌入父类基础结构体）
func (g *CodeGen) generateChildClass(decl *parser.ClassDecl, className string) {
	parentName := symbol.ToGoName(decl.Extends, true)
	parentBaseName := symbol.ToGoName(decl.Extends, false) + "Base"

	// 获取父类信息
	parentInfo := g.transpiler.table.GetClass(g.transpiler.pkg, decl.Extends)

	// 1. 生成静态字段（包级变量）
	for _, field := range decl.Fields {
		if field.Static {
			varName := fmt.Sprintf("_%s_%s", className, field.Name)
			typeName := g.generateType(field.Type)
			if field.Value != nil {
				g.writeLine(fmt.Sprintf("var %s %s = %s", varName, typeName, g.generateExpression(field.Value)))
			} else {
				g.writeLine(fmt.Sprintf("var %s %s", varName, typeName))
			}
		}
	}

	// 1.5 生成类信息变量 _ClassName_classInfo
	g.generateClassInfo(decl, className)
	g.writeLine("")

	// 2. 生成结构体（嵌入父类基础结构体）
	g.writeLine(fmt.Sprintf("type %s struct {", className))
	g.indent++

	// 如果父类是抽象类，嵌入基础结构体
	if parentInfo != nil && parentInfo.Abstract {
		g.writeLine(parentBaseName)
	} else {
		g.writeLine("*" + parentName)
	}

	// 子类自己的字段
	for _, field := range decl.Fields {
		if !field.Static {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			typeName := g.generateType(field.Type)
			g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
		}
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 3. 生成构造函数
	if decl.InitMethod != nil {
		g.generateChildClassConstructor(decl, className, parentInfo)
	} else {
		g.generateChildClassDefaultConstructor(decl, className, parentInfo)
	}
	g.writeLine("")

	// 4. 生成方法
	for _, method := range decl.Methods {
		g.generateClassMethod(decl, className, method)
		g.writeLine("")
	}

	// 4.5 生成 Class() 方法
	g.generateClassMethod_Class(className)
	g.writeLine("")
}

// generateChildClassConstructor 生成子类构造函数
func (g *CodeGen) generateChildClassConstructor(decl *parser.ClassDecl, className string, parentInfo *symbol.ClassInfo) {
	init := decl.InitMethod

	// 检查是否有默认参数
	hasDefault := false
	for _, param := range init.Params {
		if param.DefaultValue != nil {
			hasDefault = true
			break
		}
	}

	if hasDefault {
		g.generateChildClassConstructorWithDefaults(decl, className, parentInfo)
	} else {
		g.generateChildClassConstructorSimple(decl, className, parentInfo)
	}
}

// generateChildClassConstructorSimple 生成简单子类构造函数
func (g *CodeGen) generateChildClassConstructorSimple(decl *parser.ClassDecl, className string, parentInfo *symbol.ClassInfo) {
	init := decl.InitMethod
	g.writeIndent()
	g.write(fmt.Sprintf("func New__%s(", className))

	// 参数
	for i, param := range init.Params {
		if i > 0 {
			g.write(", ")
		}
		g.write(symbol.TransformDollarVar(param.Name))
		g.write(" ")
		g.write(g.generateType(param.Type))
	}

	g.write(fmt.Sprintf(") *%s {\n", className))
	g.indent++

	// 创建实例
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置父类字段默认值
	if parentInfo != nil && parentInfo.Abstract {
		for _, field := range parentInfo.Fields {
			if !field.Static && field.Value != nil {
				isPublic := field.Visibility == "public" || field.Visibility == "protected"
				fieldName := symbol.ToGoName(field.Name, isPublic)
				g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
			}
		}
	}

	// 设置子类字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	// 执行 init 方法体
	if init.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range init.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateChildClassConstructorWithDefaults 生成带默认参数的子类构造函数
func (g *CodeGen) generateChildClassConstructorWithDefaults(decl *parser.ClassDecl, className string, parentInfo *symbol.ClassInfo) {
	init := decl.InitMethod
	optsName := fmt.Sprintf("%s__InitOpts", className)

	// 生成 Opts 结构体
	g.writeLine(fmt.Sprintf("type %s struct {", optsName))
	g.indent++
	for _, param := range init.Params {
		fieldName := symbol.ToGoName(param.Name, true)
		typeName := g.generateType(param.Type)
		g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成默认值构造器
	g.writeLine(fmt.Sprintf("func NewDefault__%s() %s {", optsName, optsName))
	g.indent++
	var defaultFields []string
	for _, param := range init.Params {
		if param.DefaultValue != nil {
			fieldName := symbol.ToGoName(param.Name, true)
			defaultVal := g.generateExpression(param.DefaultValue)
			defaultFields = append(defaultFields, fmt.Sprintf("%s: %s", fieldName, defaultVal))
		}
	}
	g.writeLine(fmt.Sprintf("return %s{%s}", optsName, strings.Join(defaultFields, ", ")))
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成构造函数
	g.writeLine(fmt.Sprintf("func New__%s(opts %s) *%s {", className, optsName, className))
	g.indent++

	// 创建实例
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置父类字段默认值
	if parentInfo != nil && parentInfo.Abstract {
		for _, field := range parentInfo.Fields {
			if !field.Static && field.Value != nil {
				isPublic := field.Visibility == "public" || field.Visibility == "protected"
				fieldName := symbol.ToGoName(field.Name, isPublic)
				g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
			}
		}
	}

	// 设置子类字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	// 从 opts 获取参数
	for _, param := range init.Params {
		varName := symbol.TransformDollarVar(param.Name)
		fieldName := symbol.ToGoName(param.Name, true)
		g.writeLine(fmt.Sprintf("%s := opts.%s", varName, fieldName))
	}

	// 执行 init 方法体
	if init.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range init.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateChildClassDefaultConstructor 生成子类默认构造函数
func (g *CodeGen) generateChildClassDefaultConstructor(decl *parser.ClassDecl, className string, parentInfo *symbol.ClassInfo) {
	g.writeLine(fmt.Sprintf("func New__%s() *%s {", className, className))
	g.indent++
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置父类字段默认值
	if parentInfo != nil && parentInfo.Abstract {
		for _, field := range parentInfo.Fields {
			if !field.Static && field.Value != nil {
				isPublic := field.Visibility == "public" || field.Visibility == "protected"
				fieldName := symbol.ToGoName(field.Name, isPublic)
				g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
			}
		}
	}

	// 设置子类字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateClassConstructor 生成类构造函数
func (g *CodeGen) generateClassConstructor(decl *parser.ClassDecl, className string) {
	init := decl.InitMethod

	// 检查是否有默认参数
	hasDefault := false
	for _, param := range init.Params {
		if param.DefaultValue != nil {
			hasDefault = true
			break
		}
	}

	if hasDefault {
		g.generateClassConstructorWithDefaults(decl, className)
	} else {
		g.generateClassConstructorSimple(decl, className)
	}
}

// generateClassConstructorSimple 生成简单构造函数
func (g *CodeGen) generateClassConstructorSimple(decl *parser.ClassDecl, className string) {
	init := decl.InitMethod
	g.writeIndent()
	g.write(fmt.Sprintf("func New__%s(", className))

	// 参数
	for i, param := range init.Params {
		if i > 0 {
			g.write(", ")
		}
		g.write(symbol.TransformDollarVar(param.Name))
		g.write(" ")
		g.write(g.generateType(param.Type))
	}

	g.write(fmt.Sprintf(") *%s {\n", className))
	g.indent++

	// 创建实例
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	// 执行 init 方法体（替换 this 为 t）
	if init.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range init.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateClassConstructorWithDefaults 生成带默认参数的构造函数
func (g *CodeGen) generateClassConstructorWithDefaults(decl *parser.ClassDecl, className string) {
	init := decl.InitMethod
	optsName := fmt.Sprintf("%s__InitOpts", className)

	// 生成 Opts 结构体
	g.writeLine(fmt.Sprintf("type %s struct {", optsName))
	g.indent++
	for _, param := range init.Params {
		fieldName := symbol.ToGoName(param.Name, true)
		typeName := g.generateType(param.Type)
		g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成默认值构造器
	g.writeLine(fmt.Sprintf("func NewDefault__%s() %s {", optsName, optsName))
	g.indent++
	var defaultFields []string
	for _, param := range init.Params {
		if param.DefaultValue != nil {
			fieldName := symbol.ToGoName(param.Name, true)
			defaultVal := g.generateExpression(param.DefaultValue)
			defaultFields = append(defaultFields, fmt.Sprintf("%s: %s", fieldName, defaultVal))
		}
	}
	g.writeLine(fmt.Sprintf("return %s{%s}", optsName, strings.Join(defaultFields, ", ")))
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成构造函数
	g.writeLine(fmt.Sprintf("func New__%s(opts %s) *%s {", className, optsName, className))
	g.indent++

	// 创建实例
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	// 从 opts 获取参数
	for _, param := range init.Params {
		varName := symbol.TransformDollarVar(param.Name)
		fieldName := symbol.ToGoName(param.Name, true)
		g.writeLine(fmt.Sprintf("%s := opts.%s", varName, fieldName))
	}

	// 执行 init 方法体
	if init.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range init.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateGoMainFunc 生成 Go 的 func main()（入口类的 main 方法）
func (g *CodeGen) generateGoMainFunc(decl *parser.ClassDecl, method *parser.ClassMethod) {
	g.writeLine("func main() {")
	g.indent++

	// 方法体
	if method.Body != nil {
		// 不设置 currentReceiver，因为 main 是 static 方法，不能使用 this
		for _, stmt := range method.Body.Statements {
			g.generateStatement(stmt)
		}
	}

	g.indent--
	g.writeLine("}")
}

// generateDefaultConstructor 生成默认构造函数
func (g *CodeGen) generateDefaultConstructor(decl *parser.ClassDecl, className string) {
	g.writeLine(fmt.Sprintf("func New__%s() *%s {", className, className))
	g.indent++
	g.writeLine(fmt.Sprintf("t := &%s{}", className))

	// 设置字段默认值
	for _, field := range decl.Fields {
		if !field.Static && field.Value != nil {
			isPublic := field.Visibility == "public" || field.Visibility == "protected"
			fieldName := symbol.ToGoName(field.Name, isPublic)
			g.writeLine(fmt.Sprintf("t.%s = %s", fieldName, g.generateExpression(field.Value)))
		}
	}

	g.writeLine("return t")
	g.indent--
	g.writeLine("}")
}

// generateClassMethod 生成类方法
func (g *CodeGen) generateClassMethod(decl *parser.ClassDecl, className string, method *parser.ClassMethod) {
	isPublic := method.Visibility == "public" || method.Visibility == "protected"
	methodName := symbol.ToGoName(method.Name, isPublic)

	// 检查是否有默认参数
	hasDefault := false
	for _, param := range method.Params {
		if param.DefaultValue != nil {
			hasDefault = true
			break
		}
	}

	if hasDefault {
		g.generateClassMethodWithDefaults(className, methodName, method)
	} else {
		g.generateClassMethodSimple(className, methodName, method)
	}
}

// hasParamNameConflict 检查方法参数是否和指定名称冲突
func hasParamNameConflict(params []*parser.Field, name string) bool {
	for _, param := range params {
		if param.Name == name {
			return true
		}
	}
	return false
}

// getReceiverName 获取方法接收者名称，避免和参数名冲突
func getReceiverName(params []*parser.Field, defaultName string) string {
	if !hasParamNameConflict(params, defaultName) {
		return defaultName
	}
	// 如果默认名称冲突，尝试其他名称
	alternativeNames := []string{"this", "self", "recv", "_t", "_this", "_self"}
	for _, name := range alternativeNames {
		if !hasParamNameConflict(params, name) {
			return name
		}
	}
	// 极端情况：所有备选名称都冲突，使用带数字的名称
	for i := 0; ; i++ {
		name := fmt.Sprintf("_recv%d", i)
		if !hasParamNameConflict(params, name) {
			return name
		}
	}
}

// generateClassMethodSimple 生成简单方法
func (g *CodeGen) generateClassMethodSimple(className, methodName string, method *parser.ClassMethod) {
	// 获取原始类名（用于重载查找）
	originalClassName := g.getOriginalClassName(className)

	// 检查是否是重载方法，使用修饰后的名称
	actualMethodName := methodName
	if g.transpiler.table.IsMethodOverloaded(g.transpiler.pkg, originalClassName, method.Name) {
		isPublic := method.Visibility == "public" || method.Visibility == "protected"
		actualMethodName = symbol.GenerateMangledName(method.Name, method.Params, isPublic)
	}

	// 获取接收者名称（避免和参数名冲突）
	receiverName := getReceiverName(method.Params, "t")

	g.writeIndent()
	g.write(fmt.Sprintf("func (%s *%s) %s(", receiverName, className, actualMethodName))

	// 参数
	for i, param := range method.Params {
		if i > 0 {
			g.write(", ")
		}
		g.write(symbol.TransformDollarVar(param.Name))
		g.write(" ")
		g.write(g.generateType(param.Type))
	}
	g.write(")")

	// 返回值
	if len(method.Results) > 0 {
		g.write(" ")
		if method.Errable {
			// errable 方法：追加 error 返回值
			g.write("(")
			for i, r := range method.Results {
				if i > 0 {
					g.write(", ")
				}
				if r.Name != "" {
					g.write(symbol.TransformDollarVar(r.Name))
					g.write(" ")
				}
				g.write(g.generateType(r.Type))
			}
			g.write(", error)")
		} else if len(method.Results) == 1 && method.Results[0].Name == "" {
			g.write(g.generateType(method.Results[0].Type))
		} else {
			g.write("(")
			for i, r := range method.Results {
				if i > 0 {
					g.write(", ")
				}
				if r.Name != "" {
					g.write(symbol.TransformDollarVar(r.Name))
					g.write(" ")
				}
				g.write(g.generateType(r.Type))
			}
			g.write(")")
		}
	} else if method.Errable {
		// 无返回值但是 errable：只返回 error
		g.write(" error")
	}

	g.writeLine(" {")
	g.indent++

	// 方法体
	if method.Body != nil {
		g.currentReceiver = receiverName
		g.currentFuncErrable = method.Errable
		g.currentFuncResults = method.Results
		for _, stmt := range method.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
		g.currentFuncErrable = false
		g.currentFuncResults = nil
	}

	g.indent--
	g.writeLine("}")
}

// generateClassMethodWithDefaults 生成带默认参数的方法
func (g *CodeGen) generateClassMethodWithDefaults(className, methodName string, method *parser.ClassMethod) {
	// 获取原始类名（用于重载查找）
	originalClassName := g.getOriginalClassName(className)

	// 检查是否是重载方法，使用修饰后的名称
	actualMethodName := methodName
	if g.transpiler.table.IsMethodOverloaded(g.transpiler.pkg, originalClassName, method.Name) {
		isPublic := method.Visibility == "public" || method.Visibility == "protected"
		actualMethodName = symbol.GenerateMangledName(method.Name, method.Params, isPublic)
	}
	optsName := fmt.Sprintf("%s__%s__Opts", className, actualMethodName)

	// 获取接收者名称，避免和参数名冲突
	receiverName := getReceiverName(method.Params, "t")

	// 生成 Opts 结构体
	g.writeLine(fmt.Sprintf("type %s struct {", optsName))
	g.indent++
	for _, param := range method.Params {
		fieldName := symbol.ToGoName(param.Name, true)
		typeName := g.generateType(param.Type)
		g.writeLine(fmt.Sprintf("%s %s", fieldName, typeName))
	}
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成默认值构造器
	g.writeLine(fmt.Sprintf("func NewDefault__%s() %s {", optsName, optsName))
	g.indent++
	var defaultFields []string
	for _, param := range method.Params {
		if param.DefaultValue != nil {
			fieldName := symbol.ToGoName(param.Name, true)
			defaultVal := g.generateExpression(param.DefaultValue)
			defaultFields = append(defaultFields, fmt.Sprintf("%s: %s", fieldName, defaultVal))
		}
	}
	g.writeLine(fmt.Sprintf("return %s{%s}", optsName, strings.Join(defaultFields, ", ")))
	g.indent--
	g.writeLine("}")
	g.writeLine("")

	// 生成方法
	g.writeIndent()
	g.write(fmt.Sprintf("func (%s *%s) %s(opts %s)", receiverName, className, actualMethodName, optsName))

	// 返回值
	if len(method.Results) > 0 {
		g.write(" ")
		if method.Errable {
			// errable 方法：追加 error 返回值
			g.write("(")
			for i, r := range method.Results {
				if i > 0 {
					g.write(", ")
				}
				if r.Name != "" {
					g.write(symbol.TransformDollarVar(r.Name))
					g.write(" ")
				}
				g.write(g.generateType(r.Type))
			}
			g.write(", error)")
		} else if len(method.Results) == 1 && method.Results[0].Name == "" {
			g.write(g.generateType(method.Results[0].Type))
		} else {
			g.write("(")
			for i, r := range method.Results {
				if i > 0 {
					g.write(", ")
				}
				if r.Name != "" {
					g.write(symbol.TransformDollarVar(r.Name))
					g.write(" ")
				}
				g.write(g.generateType(r.Type))
			}
			g.write(")")
		}
	} else if method.Errable {
		// 无返回值但是 errable：只返回 error
		g.write(" error")
	}

	g.writeLine(" {")
	g.indent++

	// 从 opts 获取参数
	for _, param := range method.Params {
		varName := symbol.TransformDollarVar(param.Name)
		fieldName := symbol.ToGoName(param.Name, true)
		g.writeLine(fmt.Sprintf("%s := opts.%s", varName, fieldName))
		// 添加 _ = varName 以避免未使用变量的编译错误
		g.writeLine(fmt.Sprintf("_ = %s", varName))
	}

	// 方法体
	if method.Body != nil {
		g.currentReceiver = receiverName
		g.currentFuncErrable = method.Errable
		g.currentFuncResults = method.Results
		for _, stmt := range method.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
		g.currentFuncErrable = false
		g.currentFuncResults = nil
	}

	g.indent--
	g.writeLine("}")
}

// generateInterfaceDecl 生成接口声明
func (g *CodeGen) generateInterfaceDecl(decl *parser.InterfaceDecl) {
	name := symbol.ToGoName(decl.Name, decl.Public)
	g.writeLine(fmt.Sprintf("type %s interface {", name))
	g.indent++
	for _, method := range decl.Methods {
		methodName := symbol.ToGoName(method.Name, true) // 接口方法默认导出
		g.write(methodName + "(")
		g.generateParams(method.Params)
		g.write(")")
		if len(method.Results) > 0 {
			g.write(" ")
			if len(method.Results) == 1 && method.Results[0].Name == "" {
				g.write(g.generateType(method.Results[0].Type))
			} else {
				g.write("(")
				g.generateParams(method.Results)
				g.write(")")
			}
		}
		g.writeLine("")
	}
	g.indent--
	g.writeLine("}")
}

// generateTypeDecl 生成类型声明
func (g *CodeGen) generateTypeDecl(decl *parser.TypeDecl) {
	name := symbol.ToGoName(decl.Name, decl.Public)
	typeName := g.generateType(decl.Type)
	g.writeLine(fmt.Sprintf("type %s %s", name, typeName))
}

// generateVarDecl 生成变量声明
func (g *CodeGen) generateVarDecl(decl *parser.VarDecl) {
	names := make([]string, len(decl.Names))
	for i, name := range decl.Names {
		names[i] = symbol.TransformDollarVar(name)
	}

	var sb strings.Builder
	sb.WriteString("var ")
	sb.WriteString(strings.Join(names, ", "))

	if decl.Type != nil {
		sb.WriteString(" ")
		sb.WriteString(g.generateType(decl.Type))
	}

	if decl.Value != nil {
		sb.WriteString(" = ")
		sb.WriteString(g.generateExpression(decl.Value))
	}

	g.writeLine(sb.String())
}

// generateConstDecl 生成常量声明
func (g *CodeGen) generateConstDecl(decl *parser.ConstDecl) {
	names := make([]string, len(decl.Names))
	for i, name := range decl.Names {
		names[i] = symbol.TransformDollarVar(name)
	}

	g.write("const " + strings.Join(names, ", "))

	if decl.Type != nil {
		g.write(" " + g.generateType(decl.Type))
	}

	if decl.Value != nil {
		g.write(" = " + g.generateExpression(decl.Value))
	}

	g.writeLine("")
}

// generateShortVarDecl 生成短变量声明
func (g *CodeGen) generateShortVarDecl(decl *parser.ShortVarDecl) {
	names := make([]string, len(decl.Names))
	for i, name := range decl.Names {
		names[i] = symbol.TransformDollarVar(name)
	}

	// 跟踪变量类型（用于重载解析）
	if len(decl.Names) == 1 {
		g.trackVarType(decl.Names[0], decl.Value)
	}

	// 检查Value是否是临时的ArrayLiteral（表示多值）
	if arrLit, ok := decl.Value.(*parser.ArrayLiteral); ok && len(decl.Names) > 1 {
		// 多值赋值：a, b := 1, 2
		var values []string
		for _, elem := range arrLit.Elements {
			values = append(values, g.generateExpression(elem))
		}
		line := fmt.Sprintf("%s := %s", strings.Join(names, ", "), strings.Join(values, ", "))
		g.writeLine(line)
	} else {
		// 单值或函数调用
		line := fmt.Sprintf("%s := %s", strings.Join(names, ", "), g.generateExpression(decl.Value))
		g.writeLine(line)
	}
}

// trackVarType 跟踪变量类型
func (g *CodeGen) trackVarType(varName string, value parser.Expression) {
	switch v := value.(type) {
	case *parser.NewExpr:
		// new ClassName() -> 类型是 ClassName
		if ident, ok := v.Type.(*parser.Identifier); ok {
			g.varTypes[varName] = ident.Value
		}
	case *parser.CallExpr:
		// NewClassName() 或 NewStructName() -> 类型是类名/结构体名
		if ident, ok := v.Function.(*parser.Identifier); ok {
			funcName := ident.Value
			// 检查是否是 NewXxx 模式的构造函数调用
			if strings.HasPrefix(funcName, "New") && len(funcName) > 3 {
				typeName := funcName[3:] // 去掉 "New" 前缀
				// 尝试匹配首字母大小写不同的情况
				g.varTypes[varName] = typeName
			}
		}
	case *parser.StructLiteral:
		// StructName{} -> 类型是结构体名
		if ident, ok := v.Type.(*parser.Identifier); ok {
			g.varTypes[varName] = ident.Value
		}
	}
}

// generateAssignStmt 生成赋值语句
func (g *CodeGen) generateAssignStmt(stmt *parser.AssignStmt) {
	var left []string
	for _, expr := range stmt.Left {
		left = append(left, g.generateExpression(expr))
	}

	var right []string
	for _, expr := range stmt.Right {
		right = append(right, g.generateExpression(expr))
	}

	g.writeLine(strings.Join(left, ", ") + " " + stmt.Token.Literal + " " + strings.Join(right, ", "))
}

// generateReturnStmt 生成 return 语句
func (g *CodeGen) generateReturnStmt(stmt *parser.ReturnStmt) {
	if len(stmt.Values) == 0 {
		if g.currentFuncErrable {
			// errable 函数无返回值：return nil
			g.writeLine("return nil")
		} else {
			g.writeLine("return")
		}
		return
	}

	var values []string
	for _, v := range stmt.Values {
		values = append(values, g.generateExpression(v))
	}

	if g.currentFuncErrable {
		// 检查是否返回的是一个errable调用
		// 如果只有一个返回值且是errable调用，则不追加nil（错误会自动传播）
		if len(stmt.Values) == 1 && g.isErrableCall(stmt.Values[0]) {
			g.writeLine("return " + strings.Join(values, ", "))
		} else {
			// errable 函数：追加 nil
			g.writeLine("return " + strings.Join(values, ", ") + ", nil")
		}
	} else {
		g.writeLine("return " + strings.Join(values, ", "))
	}
}

// generateThrowStmt 生成 throw 语句
func (g *CodeGen) generateThrowStmt(stmt *parser.ThrowStmt) {
	// throw expr 翻译为 return zeroValues, expr
	// 需要根据当前函数的返回值数量生成零值
	errorExpr := g.generateExpression(stmt.Value)
	
	// 简化处理：假设当前在 errable 函数中
	// 生成零值（可以根据实际返回类型优化）
	g.writeLine("return " + g.generateZeroValues() + errorExpr)
}

// generateZeroValues 生成零值列表（用于 throw）
func (g *CodeGen) generateZeroValues() string {
	if len(g.currentFuncResults) == 0 {
		return ""
	}
	
	var zeroValues []string
	for _, result := range g.currentFuncResults {
		zeroValue := g.getTypeZeroValue(result.Type)
		zeroValues = append(zeroValues, zeroValue)
	}
	
	return strings.Join(zeroValues, ", ") + ", "
}

// getTypeZeroValue 根据类型返回零值
func (g *CodeGen) getTypeZeroValue(typ parser.Expression) string {
	if typ == nil {
		return "nil"
	}
	
	// 获取类型名称
	typeName := ""
	if ident, ok := typ.(*parser.Identifier); ok {
		typeName = ident.Value
	} else {
		// 复杂类型（指针、切片、map等）默认返回 nil
		return "nil"
	}
	
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		return "0"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "0"
	case "float32", "float64":
		return "0.0"
	case "bool":
		return "false"
	case "string":
		return "\"\""
	case "byte", "rune":
		return "0"
	default:
		// 指针、接口、切片、map 等引用类型
		return "nil"
	}
}

// generateTryStmt 生成 try-catch 语句
func (g *CodeGen) generateTryStmt(stmt *parser.TryStmt) {
	// 检测 try 块内是否有需要错误处理的代码
	needsErrorHandling := g.tryBlockNeedsErrorHandling(stmt.Body.Statements)

	if !needsErrorHandling {
		// 简化模式：try 块内没有 throw 或 errable 调用，直接生成普通代码块
		g.writeLine("{")
		g.indent++
		for _, s := range stmt.Body.Statements {
			g.generateStatement(s)
		}
		g.indent--
		g.writeLine("}")
		// catch 块不会被执行，可以省略
		return
	}

	// 完整模式：需要错误处理机制
	g.tryCounter++
	labelName := fmt.Sprintf("_TryBlock_%d", g.tryCounter)
	errVarName := fmt.Sprintf("_tryErr_%d", g.tryCounter)

	// 开始一个新的作用域
	g.writeLine("{")
	g.indent++

	// 声明错误变量
	g.writeLine(fmt.Sprintf("var %s error", errVarName))

	// 生成 labeled for 循环
	g.writeLine(fmt.Sprintf("%s:", labelName))
	g.writeLine("for _once := true; _once; _once = false {")
	g.indent++

	// 设置 inTryBlock 标志
	oldInTryBlock := g.inTryBlock
	g.inTryBlock = true

	// 生成 try 块中的语句
	// 这里需要特殊处理，为 errable 调用插入错误检查
	tmpCounter := 0
	g.generateTryBlockStatements(stmt.Body.Statements, labelName, errVarName, &tmpCounter)

	g.inTryBlock = oldInTryBlock
	g.indent--
	g.writeLine("}")

	// 生成 catch 块
	if stmt.Catch != nil {
		g.writeLine(fmt.Sprintf("if %s != nil {", errVarName))
		g.indent++

		// 将错误赋值给 catch 参数
		if stmt.Catch.Param != "" {
			g.writeLine(fmt.Sprintf("%s := %s", stmt.Catch.Param, errVarName))
		}

		// 生成 catch 块体
		if stmt.Catch.Body != nil {
			for _, s := range stmt.Catch.Body.Statements {
				g.generateStatement(s)
			}
		}

		g.indent--
		g.writeLine("}")
	}

	g.indent--
	g.writeLine("}")
	// tryCounter持续递增，不需要递减
}

// tryBlockNeedsErrorHandling 检测 try 块内是否有需要错误处理的代码
// 返回 true 如果有 throw 语句或 errable 函数调用
func (g *CodeGen) tryBlockNeedsErrorHandling(statements []parser.Statement) bool {
	for _, stmt := range statements {
		if g.statementNeedsErrorHandling(stmt) {
			return true
		}
	}
	return false
}

// statementNeedsErrorHandling 递归检测语句是否需要错误处理
func (g *CodeGen) statementNeedsErrorHandling(stmt parser.Statement) bool {
	switch s := stmt.(type) {
	case *parser.ThrowStmt:
		// throw 语句需要错误处理
		return true
	case *parser.ShortVarDecl:
		// 检查右侧是否有 errable 调用
		if g.isErrableCall(s.Value) || g.containsErrableCall(s.Value) {
			return true
		}
	case *parser.AssignStmt:
		// 检查右侧是否有 errable 调用
		for _, expr := range s.Right {
			if g.isErrableCall(expr) || g.containsErrableCall(expr) {
				return true
			}
		}
	case *parser.ExpressionStmt:
		// 检查表达式是否有 errable 调用
		if g.isErrableCall(s.Expression) || g.containsErrableCall(s.Expression) {
			return true
		}
	case *parser.IfStmt:
		// 递归检查 if 块
		if s.Consequence != nil {
			if g.tryBlockNeedsErrorHandling(s.Consequence.Statements) {
				return true
			}
		}
		// Alternative 可能是 BlockStmt 或 IfStmt（else if）
		if s.Alternative != nil {
			if g.statementNeedsErrorHandling(s.Alternative) {
				return true
			}
		}
	case *parser.ForStmt:
		// 递归检查 for 块
		if s.Body != nil {
			if g.tryBlockNeedsErrorHandling(s.Body.Statements) {
				return true
			}
		}
	case *parser.RangeStmt:
		// 递归检查 range 块
		if s.Body != nil {
			if g.tryBlockNeedsErrorHandling(s.Body.Statements) {
				return true
			}
		}
	case *parser.BlockStmt:
		// 递归检查代码块
		if g.tryBlockNeedsErrorHandling(s.Statements) {
			return true
		}
	}
	return false
}

// generateTryBlockStatements 生成 try 块中的语句，为 errable 调用插入错误检查
func (g *CodeGen) generateTryBlockStatements(statements []parser.Statement, labelName string, errVarName string, tmpCounter *int) {
	for _, stmt := range statements {
		g.generateTryBlockStatement(stmt, labelName, errVarName, tmpCounter)
	}
}

// generateTryBlockStatement 生成 try 块中的单个语句
func (g *CodeGen) generateTryBlockStatement(stmt parser.Statement, labelName string, errVarName string, tmpCounter *int) {
	switch s := stmt.(type) {
	case *parser.ShortVarDecl:
		// 短变量声明：a, b := errableFunc()
		// 需要检查右侧是否是 errable 调用
		if g.isErrableCall(s.Value) {
			// 获取函数返回值数量
			resultCount := g.getErrableFuncResultCount(s.Value.(*parser.CallExpr))
			
			// 检查用户接收的变量数量是否匹配
			if len(s.Names) != resultCount {
				// 变量数量不匹配，可能有 _ 忽略符，这是允许的
				// 但不能超过返回值数量
				if len(s.Names) > resultCount {
					g.transpiler.errors = append(g.transpiler.errors, i18n.T(i18n.ErrTooManyVariables,
						resultCount, len(s.Names)))
					g.generateStatement(s)
					return
				}
			}
			
			// 生成带错误检查的调用
			names := make([]string, len(s.Names))
			for i, name := range s.Names {
				names[i] = symbol.TransformDollarVar(name)
			}
			
			// 追加 _err
			line := fmt.Sprintf("%s, _err := %s", strings.Join(names, ", "), g.generateExpression(s.Value))
			g.writeLine(line)

			// 插入错误检查
			g.writeLine("if _err != nil {")
			g.indent++
			g.writeLine(fmt.Sprintf("%s = _err", errVarName))
			g.writeLine(fmt.Sprintf("break %s", labelName))
			g.indent--
			g.writeLine("}")
		} else {
			g.generateStatement(s)
		}
	case *parser.AssignStmt:
		// 赋值语句：检查右侧是否有 errable 调用
		hasErrableCall := false
		for _, expr := range s.Right {
			if g.isErrableCall(expr) {
				hasErrableCall = true
				break
			}
		}

		if hasErrableCall {
			// 生成带错误检查的赋值
			var left []string
			for _, expr := range s.Left {
				left = append(left, g.generateExpression(expr))
			}

			g.writeIndent()
			g.write(strings.Join(left, ", "))
			g.write(", _err ")
			g.write(s.Token.Literal)
			g.write(" ")

			var right []string
			for _, expr := range s.Right {
				right = append(right, g.generateExpression(expr))
			}
			g.write(strings.Join(right, ", "))
			g.write("\n")

			// 插入错误检查
			g.writeLine("if _err != nil {")
			g.indent++
			g.writeLine(fmt.Sprintf("%s = _err", errVarName))
			g.writeLine(fmt.Sprintf("break %s", labelName))
			g.indent--
			g.writeLine("}")
		} else {
			g.generateStatement(s)
		}
	case *parser.ExpressionStmt:
		// 表达式语句：可能是单独的函数调用或包含嵌套 errable 调用
		if g.isErrableCall(s.Expression) {
			// 顶层就是 errable 调用
			call := s.Expression.(*parser.CallExpr)
			resultCount := g.getErrableFuncResultCount(call)
			
			if resultCount > 1 {
				// 多返回值函数不能直接作为表达式语句，需要接收返回值
				g.transpiler.errors = append(g.transpiler.errors, i18n.T(i18n.ErrErrableMultiReturnNoAssign,
					resultCount))
				g.generateStatement(s)
				return
			}
			
			// 单返回值，生成带错误检查的调用
			g.writeIndent()
			if resultCount == 1 {
				g.write("_, _err := ")
			} else {
				g.write("_err := ")
			}
			g.write(g.generateExpression(s.Expression))
			g.write("\n")

			// 插入错误检查
			g.writeLine("if _err != nil {")
			g.indent++
			g.writeLine(fmt.Sprintf("%s = _err", errVarName))
			g.writeLine(fmt.Sprintf("break %s", labelName))
			g.indent--
			g.writeLine("}")
		} else if g.containsErrableCall(s.Expression) {
			// 包含嵌套的 errable 调用，需要提取（包括多返回值的情况）
			newExpr := g.extractErrableCalls(s.Expression, labelName, errVarName, tmpCounter)
			// 生成替换后的表达式语句
			g.writeIndent()
			g.write(g.generateExpressionFromExtracted(newExpr))
			g.write("\n")
		} else {
			g.generateStatement(s)
		}
	case *parser.ThrowStmt:
		// 在 try 块内的 throw，设置错误变量并 break
		errorExpr := g.generateExpression(s.Value)
		g.writeLine(fmt.Sprintf("%s = %s", errVarName, errorExpr))
		g.writeLine(fmt.Sprintf("break %s", labelName))
	default:
		// 其他语句直接生成
		g.generateStatement(s)
	}
}

// isErrableCall 检查表达式是否是 errable 函数调用
func (g *CodeGen) isErrableCall(expr parser.Expression) bool {
	if call, ok := expr.(*parser.CallExpr); ok {
		// 检查被调用的函数是否是 errable
		if ident, ok := call.Function.(*parser.Identifier); ok {
			sym := g.transpiler.table.Get(g.transpiler.pkg, ident.Value)
			if sym != nil && sym.Errable {
				return true
			}
		} else if sel, ok := call.Function.(*parser.SelectorExpr); ok {
			// 方法调用（不区分大小写，因为tugo的getName会变成Go的GetName）
			methodName := sel.Sel
			methodNameLower := strings.ToLower(methodName)
			for _, sym := range g.transpiler.table.GetAll() {
				if (sym.Kind == symbol.SymbolClassMethod || sym.Kind == symbol.SymbolMethod) && 
				   strings.ToLower(sym.Name) == methodNameLower && sym.Errable {
					return true
				}
			}
		}
	}
	return false
}

// containsErrableCall 递归检查表达式是否包含 errable 调用
func (g *CodeGen) containsErrableCall(expr parser.Expression) bool {
	if expr == nil {
		return false
	}
	
	// 先检查当前表达式
	if g.isErrableCall(expr) {
		return true
	}
	
	// 递归检查子表达式
	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查参数
		for _, arg := range e.Arguments {
			if g.containsErrableCall(arg) {
				return true
			}
		}
	case *parser.BinaryExpr:
		return g.containsErrableCall(e.Left) || g.containsErrableCall(e.Right)
	case *parser.UnaryExpr:
		return g.containsErrableCall(e.Operand)
	case *parser.IndexExpr:
		return g.containsErrableCall(e.X) || g.containsErrableCall(e.Index)
	case *parser.SelectorExpr:
		return g.containsErrableCall(e.X)
	}
	
	return false
}

// extractedExpr 表示提取后的表达式（临时变量名）
type extractedExpr struct {
	varName string
}

func (e *extractedExpr) TokenLiteral() string { return e.varName }
func (e *extractedExpr) expressionNode()      {}

// extractErrableCalls 递归提取 errable 调用，生成临时变量和错误检查
// 返回替换后的表达式（errable 调用被替换为临时变量标识符）
func (g *CodeGen) extractErrableCalls(expr parser.Expression, labelName, errVarName string, tmpCounter *int) parser.Expression {
	if expr == nil {
		return nil
	}
	
	switch e := expr.(type) {
	case *parser.CallExpr:
		// 先检查当前调用是否是 errable
		if g.isErrableCall(e) {
			// 获取函数的返回值数量
			resultCount := g.getErrableFuncResultCount(e)
			
			// 为每个返回值生成临时变量
			var tmpVars []string
			for i := 0; i < resultCount; i++ {
				*tmpCounter++
				tmpVars = append(tmpVars, fmt.Sprintf("_tmp%d", *tmpCounter))
			}
			
			// 生成提取代码
			g.writeIndent()
			if len(tmpVars) > 0 {
				g.write(strings.Join(tmpVars, ", "))
				g.write(", _err := ")
			} else {
				g.write("_err := ")
			}
			g.write(g.generateExpression(e))
			g.write("\n")
			
			// 生成错误检查
			g.writeLine("if _err != nil {")
			g.indent++
			g.writeLine(fmt.Sprintf("%s = _err", errVarName))
			g.writeLine(fmt.Sprintf("break %s", labelName))
			g.indent--
			g.writeLine("}")
			
			// 对于多返回值，创建一个特殊的"多值标识符"（用逗号连接）
			// 这会在参数展开时被特殊处理
			if len(tmpVars) > 0 {
				if len(tmpVars) == 1 {
					return &parser.Identifier{Value: tmpVars[0]}
				}
				// 多个返回值：用特殊标记表示需要展开
				return &parser.Identifier{Value: "$$MULTI$$" + strings.Join(tmpVars, ",")}
			}
			return &parser.Identifier{Value: "_"}
		}
		
		// 不是 errable，但参数中可能有 errable 调用
		newArgs := make([]parser.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			newArgs[i] = g.extractErrableCalls(arg, labelName, errVarName, tmpCounter)
		}
		
		// 返回新的 CallExpr（参数被替换）
		return &parser.CallExpr{
			Function:  e.Function,
			Arguments: newArgs,
		}
		
	case *parser.BinaryExpr:
		return &parser.BinaryExpr{
			Left:     g.extractErrableCalls(e.Left, labelName, errVarName, tmpCounter),
			Operator: e.Operator,
			Right:    g.extractErrableCalls(e.Right, labelName, errVarName, tmpCounter),
		}
		
	case *parser.UnaryExpr:
		return &parser.UnaryExpr{
			Operator: e.Operator,
			Operand:  g.extractErrableCalls(e.Operand, labelName, errVarName, tmpCounter),
		}
		
	default:
		// 其他类型的表达式直接返回
		return expr
	}
}

// generateExpressionFromExtracted 生成提取后的表达式（可能包含临时变量）
func (g *CodeGen) generateExpressionFromExtracted(expr parser.Expression) string {
	if expr == nil {
		return ""
	}
	
	// 对于临时变量标识符，直接返回变量名
	if ident, ok := expr.(*parser.Identifier); ok {
		// 检查是否是多值标识符
		if strings.HasPrefix(ident.Value, "$$MULTI$$") {
			// 去除标记前缀，返回逗号分隔的变量
			return strings.TrimPrefix(ident.Value, "$$MULTI$$")
		}
		return ident.Value
	}
	
	// 对于 CallExpr，需要特殊处理参数中的多值展开
	if call, ok := expr.(*parser.CallExpr); ok {
		return g.generateCallExprWithMultiValueExpansion(call)
	}
	
	// 其他情况使用标准生成
	return g.generateExpression(expr)
}

// generateCallExprWithMultiValueExpansion 生成 CallExpr，展开多值参数
func (g *CodeGen) generateCallExprWithMultiValueExpansion(call *parser.CallExpr) string {
	var result strings.Builder
	
	// 生成函数名
	result.WriteString(g.generateExpression(call.Function))
	result.WriteString("(")
	
	// 处理参数，展开多值标识符
	var expandedArgs []string
	for _, arg := range call.Arguments {
		if ident, ok := arg.(*parser.Identifier); ok && strings.HasPrefix(ident.Value, "$$MULTI$$") {
			// 展开多值参数
			values := strings.TrimPrefix(ident.Value, "$$MULTI$$")
			expandedArgs = append(expandedArgs, strings.Split(values, ",")...)
		} else {
			// 普通参数
			expandedArgs = append(expandedArgs, g.generateExpressionFromExtracted(arg))
		}
	}
	
	result.WriteString(strings.Join(expandedArgs, ", "))
	result.WriteString(")")
	
	return result.String()
}

// generateErrableCallStmt 生成 errable 调用语句（自动错误检查和传播）
func (g *CodeGen) generateErrableCallStmt(expr parser.Expression) {
	call, ok := expr.(*parser.CallExpr)
	if !ok {
		g.writeLine(g.generateExpression(expr))
		return
	}
	
	resultCount := g.getErrableFuncResultCount(call)
	
	// 生成接收变量（全部用 _ 忽略，只关心 error）
	g.writeIndent()
	for i := 0; i < resultCount; i++ {
		g.write("_, ")
	}
	g.write("err := ")
	g.write(g.generateExpression(call))
	g.write("\n")
	
	// 生成错误检查
	g.writeLine("if err != nil {")
	g.indent++
	
	// 返回零值 + error
	g.writeIndent()
	g.write("return ")
	zeroVals := g.generateZeroValues()
	if zeroVals != "" {
		g.write(zeroVals)
	}
	g.write("err\n")
	
	g.indent--
	g.writeLine("}")
}

// hasMultiValueErrableCall 检查表达式中是否包含返回多值的 errable 调用
func (g *CodeGen) hasMultiValueErrableCall(expr parser.Expression) bool {
	if expr == nil {
		return false
	}
	
	if call, ok := expr.(*parser.CallExpr); ok {
		if g.isErrableCall(call) {
			return g.getErrableFuncResultCount(call) > 1
		}
		// 检查参数
		for _, arg := range call.Arguments {
			if g.hasMultiValueErrableCall(arg) {
				return true
			}
		}
	}
	
	// 递归检查子表达式
	switch e := expr.(type) {
	case *parser.BinaryExpr:
		return g.hasMultiValueErrableCall(e.Left) || g.hasMultiValueErrableCall(e.Right)
	case *parser.UnaryExpr:
		return g.hasMultiValueErrableCall(e.Operand)
	}
	
	return false
}

// getErrableFuncResultCount 获取 errable 函数的返回值数量（不包括 error）
func (g *CodeGen) getErrableFuncResultCount(call *parser.CallExpr) int {
	if ident, ok := call.Function.(*parser.Identifier); ok {
		// 函数调用
		funcName := ident.Value
		sym := g.transpiler.table.Get(g.transpiler.pkg, funcName)
		if sym != nil && sym.Errable {
			return sym.ResultCount
		}
	} else if sel, ok := call.Function.(*parser.SelectorExpr); ok {
		// 方法调用 obj.method()
		methodName := sel.Sel
		
		// 尝试从所有类方法中查找（不区分大小写）
		methodNameLower := strings.ToLower(methodName)
		
		for _, sym := range g.transpiler.table.GetAll() {
			if (sym.Kind == symbol.SymbolClassMethod || sym.Kind == symbol.SymbolMethod) &&
				strings.ToLower(sym.Name) == methodNameLower && sym.Errable {
				return sym.ResultCount
			}
		}
	}
	
	// 默认返回1（保守处理）
	return 1
}

// generateIfStmt 生成 if 语句
func (g *CodeGen) generateIfStmt(stmt *parser.IfStmt) {
	g.writeIndent()
	g.write("if ")

	if stmt.Init != nil {
		if exprStmt, ok := stmt.Init.(*parser.ExpressionStmt); ok {
			g.write(g.generateExpression(exprStmt.Expression))
			g.write("; ")
		}
	}

	g.write(g.generateExpression(stmt.Condition))
	g.write(" ")
	g.generateBlockStmtInline(stmt.Consequence)

	if stmt.Alternative != nil {
		g.write(" else ")
		switch alt := stmt.Alternative.(type) {
		case *parser.BlockStmt:
			g.generateBlockStmtInline(alt)
		case *parser.IfStmt:
			g.generateIfStmtInline(alt)
			g.builder.WriteString("\n")
			return
		}
	}
	g.builder.WriteString("\n")
}

// generateIfStmtInline 生成内联 if 语句（用于 else if）
func (g *CodeGen) generateIfStmtInline(stmt *parser.IfStmt) {
	g.write("if ")

	if stmt.Init != nil {
		if exprStmt, ok := stmt.Init.(*parser.ExpressionStmt); ok {
			g.write(g.generateExpression(exprStmt.Expression))
			g.write("; ")
		}
	}

	g.write(g.generateExpression(stmt.Condition))
	g.write(" ")
	g.generateBlockStmtInline(stmt.Consequence)

	if stmt.Alternative != nil {
		g.write(" else ")
		switch alt := stmt.Alternative.(type) {
		case *parser.BlockStmt:
			g.generateBlockStmtInline(alt)
		case *parser.IfStmt:
			g.generateIfStmtInline(alt)
		}
	}
}

// generateForStmt 生成 for 语句
func (g *CodeGen) generateForStmt(stmt *parser.ForStmt) {
	g.writeIndent()
	g.write("for ")

	if stmt.Init != nil || stmt.Post != nil {
		// 三段式 for 循环
		if stmt.Init != nil {
			if exprStmt, ok := stmt.Init.(*parser.ExpressionStmt); ok {
				g.write(g.generateExpression(exprStmt.Expression))
			}
		}
		g.write("; ")
		if stmt.Condition != nil {
			g.write(g.generateExpression(stmt.Condition))
		}
		g.write("; ")
		if stmt.Post != nil {
			if exprStmt, ok := stmt.Post.(*parser.ExpressionStmt); ok {
				g.write(g.generateExpression(exprStmt.Expression))
			}
		}
	} else if stmt.Condition != nil {
		g.write(g.generateExpression(stmt.Condition))
	}

	g.write(" ")
	g.generateBlockStmtInline(stmt.Body)
	g.builder.WriteString("\n")
}

// generateRangeStmt 生成 range 语句
func (g *CodeGen) generateRangeStmt(stmt *parser.RangeStmt) {
	g.writeIndent()
	g.write("for ")

	if stmt.Key != nil {
		g.write(g.generateExpression(stmt.Key))
		if stmt.Value != nil {
			g.write(", ")
			g.write(g.generateExpression(stmt.Value))
		}
		g.write(" := ")
	}

	g.write("range ")
	g.write(g.generateExpression(stmt.X))
	g.write(" ")
	g.generateBlockStmtInline(stmt.Body)
	g.builder.WriteString("\n")
}

// generateSwitchStmt 生成 switch 语句
func (g *CodeGen) generateSwitchStmt(stmt *parser.SwitchStmt) {
	g.write("switch ")

	if stmt.Init != nil {
		if exprStmt, ok := stmt.Init.(*parser.ExpressionStmt); ok {
			g.write(g.generateExpression(exprStmt.Expression))
			g.write("; ")
		}
	}

	if stmt.Tag != nil {
		g.write(g.generateExpression(stmt.Tag))
	}

	g.writeLine(" {")
	for _, c := range stmt.Cases {
		g.generateCaseClause(c)
	}
	g.writeLine("}")
}

// generateCaseClause 生成 case 子句
func (g *CodeGen) generateCaseClause(clause *parser.CaseClause) {
	if len(clause.Exprs) == 0 {
		g.writeLine("default:")
	} else {
		var exprs []string
		for _, e := range clause.Exprs {
			exprs = append(exprs, g.generateExpression(e))
		}
		g.writeLine("case " + strings.Join(exprs, ", ") + ":")
	}

	g.indent++
	for _, stmt := range clause.Body {
		g.generateStatement(stmt)
	}
	g.indent--
}

// generateSelectStmt 生成 select 语句
func (g *CodeGen) generateSelectStmt(stmt *parser.SelectStmt) {
	g.writeLine("select {")
	for _, c := range stmt.Cases {
		g.generateCommClause(c)
	}
	g.writeLine("}")
}

// generateCommClause 生成通信子句
func (g *CodeGen) generateCommClause(clause *parser.CommClause) {
	if clause.Comm == nil {
		g.writeLine("default:")
	} else {
		g.write("case ")
		switch s := clause.Comm.(type) {
		case *parser.ExpressionStmt:
			g.write(g.generateExpression(s.Expression))
		case *parser.SendStmt:
			g.write(g.generateExpression(s.Channel))
			g.write(" <- ")
			g.write(g.generateExpression(s.Value))
		case *parser.ShortVarDecl:
			names := make([]string, len(s.Names))
			for i, name := range s.Names {
				names[i] = symbol.TransformDollarVar(name)
			}
			g.write(strings.Join(names, ", "))
			g.write(" := ")
			g.write(g.generateExpression(s.Value))
		case *parser.AssignStmt:
			var left []string
			for _, expr := range s.Left {
				left = append(left, g.generateExpression(expr))
			}
			var right []string
			for _, expr := range s.Right {
				right = append(right, g.generateExpression(expr))
			}
			g.write(strings.Join(left, ", "))
			g.write(" = ")
			g.write(strings.Join(right, ", "))
		}
		g.writeLine(":")
	}

	g.indent++
	for _, stmt := range clause.Body {
		g.generateStatement(stmt)
	}
	g.indent--
}

// generateGoStmt 生成 go 语句
func (g *CodeGen) generateGoStmt(stmt *parser.GoStmt) {
	if stmt.Call != nil {
		g.writeLine("go " + g.generateExpression(stmt.Call))
	}
}

// generateDeferStmt 生成 defer 语句
func (g *CodeGen) generateDeferStmt(stmt *parser.DeferStmt) {
	if stmt.Call != nil {
		g.writeLine("defer " + g.generateExpression(stmt.Call))
	}
}

// generateBreakStmt 生成 break 语句
func (g *CodeGen) generateBreakStmt(stmt *parser.BreakStmt) {
	if stmt.Label != "" {
		g.writeLine("break " + stmt.Label)
	} else {
		g.writeLine("break")
	}
}

// generateContinueStmt 生成 continue 语句
func (g *CodeGen) generateContinueStmt(stmt *parser.ContinueStmt) {
	if stmt.Label != "" {
		g.writeLine("continue " + stmt.Label)
	} else {
		g.writeLine("continue")
	}
}

// generateBlockStmt 生成代码块
func (g *CodeGen) generateBlockStmt(stmt *parser.BlockStmt) {
	g.writeLine("{")
	g.indent++
	for _, s := range stmt.Statements {
		g.generateStatement(s)
	}
	g.indent--
	g.writeLine("}")
}

// generateBlockStmtInline 生成内联代码块
func (g *CodeGen) generateBlockStmtInline(stmt *parser.BlockStmt) {
	g.write("{")
	if len(stmt.Statements) == 0 {
		g.write("}")
		return
	}
	g.writeLine("")
	g.indent++
	for _, s := range stmt.Statements {
		g.generateStatement(s)
	}
	g.indent--
	g.writeIndent()
	g.write("}")
}

// generateSendStmt 生成发送语句
func (g *CodeGen) generateSendStmt(stmt *parser.SendStmt) {
	g.writeLine(g.generateExpression(stmt.Channel) + " <- " + g.generateExpression(stmt.Value))
}

// generateIncDecStmt 生成自增/自减语句
func (g *CodeGen) generateIncDecStmt(stmt *parser.IncDecStmt) {
	expr := g.generateExpression(stmt.X)
	if stmt.Inc {
		g.writeLine(expr + "++")
	} else {
		g.writeLine(expr + "--")
	}
}

// generateExpression 生成表达式
func (g *CodeGen) generateExpression(expr parser.Expression) string {
	if expr == nil {
		return ""
	}

	switch e := expr.(type) {
	case *parser.Identifier:
		return g.generateIdentifier(e)
	case *parser.IntegerLiteral:
		return e.Value
	case *parser.FloatLiteral:
		return e.Value
	case *parser.StringLiteral:
		return e.Value
	case *parser.CharLiteral:
		return e.Value
	case *parser.BoolLiteral:
		if e.Value {
			return "true"
		}
		return "false"
	case *parser.NilLiteral:
		return "nil"
	case *parser.ThisExpr:
		if g.currentReceiver != "" {
			return g.currentReceiver
		}
		return "this" // 不应该发生
	case *parser.SelfExpr:
		return "self" // 只会在 StaticAccessExpr 中使用
	case *parser.StaticAccessExpr:
		return g.generateStaticAccessExpr(e)
	case *parser.BinaryExpr:
		return g.generateBinaryExpr(e)
	case *parser.UnaryExpr:
		return g.generateUnaryExpr(e)
	case *parser.CallExpr:
		return g.generateCallExpr(e)
	case *parser.IndexExpr:
		return g.generateIndexExpr(e)
	case *parser.SliceExpr:
		return g.generateSliceExpr(e)
	case *parser.SelectorExpr:
		return g.generateSelectorExpr(e)
	case *parser.TypeAssertExpr:
		return g.generateTypeAssertExpr(e)
	case *parser.ParenExpr:
		return "(" + g.generateExpression(e.X) + ")"
	case *parser.ArrayLiteral:
		return g.generateArrayLiteral(e)
	case *parser.SliceLiteral:
		return g.generateSliceLiteral(e)
	case *parser.MapLiteral:
		return g.generateMapLiteral(e)
	case *parser.StructLiteral:
		return g.generateStructLiteral(e)
	case *parser.FuncLiteral:
		return g.generateFuncLiteral(e)
	case *parser.ReceiveExpr:
		return "<-" + g.generateExpression(e.X)
	case *parser.MakeExpr:
		return g.generateMakeExpr(e)
	case *parser.NewExpr:
		return g.generateNewExpr(e)
	case *parser.LenExpr:
		return "len(" + g.generateExpression(e.X) + ")"
	case *parser.CapExpr:
		return "cap(" + g.generateExpression(e.X) + ")"
	case *parser.AppendExpr:
		return g.generateAppendExpr(e)
	case *parser.CopyExpr:
		return "copy(" + g.generateExpression(e.Dst) + ", " + g.generateExpression(e.Src) + ")"
	case *parser.DeleteExpr:
		return "delete(" + g.generateExpression(e.Map) + ", " + g.generateExpression(e.Key) + ")"
	case *parser.ArrayType:
		return g.generateArrayType(e)
	case *parser.SliceType:
		return "[]" + g.generateType(e.Elt)
	case *parser.MapType:
		return "map[" + g.generateType(e.Key) + "]" + g.generateType(e.Value)
	case *parser.ChanType:
		return g.generateChanType(e)
	case *parser.PointerType:
		return "*" + g.generateType(e.Base)
	case *parser.FuncType:
		return g.generateFuncType(e)
	case *parser.InterfaceType:
		return g.generateInterfaceType(e)
	case *parser.StructType:
		return g.generateStructType(e)
	case *parser.Ellipsis:
		return "..." + g.generateType(e.Elt)
	default:
		return ""
	}
}

// generateIdentifier 生成标识符
func (g *CodeGen) generateIdentifier(ident *parser.Identifier) string {
	name := ident.Value

	// 处理 $ 开头的变量名
	if strings.HasPrefix(name, "$") {
		return symbol.TransformDollarVar(name)
	}

	// 检查是否是导入的类型名
	if pkgName, ok := g.typeToPackage[name]; ok {
		return pkgName + "." + name
	}

	// 查找符号表
	sym := g.transpiler.LookupSymbol(name)
	if sym != nil {
		return sym.GoName
	}

	return name
}

// generateBinaryExpr 生成二元表达式
func (g *CodeGen) generateBinaryExpr(expr *parser.BinaryExpr) string {
	left := g.generateExpression(expr.Left)
	right := g.generateExpression(expr.Right)
	return left + " " + expr.Operator + " " + right
}

// generateUnaryExpr 生成一元表达式
func (g *CodeGen) generateUnaryExpr(expr *parser.UnaryExpr) string {
	operand := g.generateExpression(expr.Operand)
	return expr.Operator + operand
}

// generateCallExpr 生成函数调用表达式
func (g *CodeGen) generateCallExpr(expr *parser.CallExpr) string {
	// 检查是否是全局函数
	if ident, ok := expr.Function.(*parser.Identifier); ok {
		switch ident.Value {
		case "print":
			return g.generatePrintCall("Print", expr.Arguments)
		case "println":
			return g.generatePrintCall("Println", expr.Arguments)
		case "print_f":
			return g.generatePrintCall("Printf", expr.Arguments)
		case "errorf":
			// errorf 翻译为 fmt.Errorf
			g.transpiler.needFmt = true
			var argStrs []string
			for _, arg := range expr.Arguments {
				argStrs = append(argStrs, g.generateExpression(arg))
			}
			return "fmt.Errorf(" + strings.Join(argStrs, ", ") + ")"
		}

		// 检查是否是带默认参数的函数
		sym := g.transpiler.LookupSymbol(ident.Value)
		if sym != nil && sym.HasDefault {
			return g.generateDefaultParamCall(sym, expr.Arguments)
		}
	}

	// 检查是否是方法调用（obj.method()）且方法有重载
	if sel, ok := expr.Function.(*parser.SelectorExpr); ok {
		methodName := sel.Sel
		
		// 尝试获取接收者类型来查找重载
		receiverType := g.getReceiverType(sel.X)
		if receiverType != "" {
			// 确定类所在的包（可能是当前包或导入的包）
			classPkg := g.getClassPackage(receiverType)
			
			// 检查是否有重载
			if g.transpiler.table.IsMethodOverloaded(classPkg, receiverType, methodName) {
				// 解析重载方法，同时获取方法信息
				mangledName, overloadMethod := g.resolveOverloadedMethodWithInfo(classPkg, receiverType, methodName, expr.Arguments)
				x := g.generateExpression(sel.X)
				
				// 检查是否需要使用 Opts 模式
				if overloadMethod != nil && overloadMethod.HasDefaults {
					// 生成 Opts 结构体调用
					return g.generateOptsCall(x, classPkg, receiverType, mangledName, overloadMethod, expr.Arguments)
				}
				
				var args []string
				for _, arg := range expr.Arguments {
					args = append(args, g.generateExpression(arg))
				}
				return x + "." + mangledName + "(" + strings.Join(args, ", ") + ")"
			}
		}
	}

	funcExpr := g.generateExpression(expr.Function)
	var args []string
	for _, arg := range expr.Arguments {
		args = append(args, g.generateExpression(arg))
	}
	return funcExpr + "(" + strings.Join(args, ", ") + ")"
}

// getOriginalClassName 获取原始类名（Go名称 -> Tugo名称）
func (g *CodeGen) getOriginalClassName(goClassName string) string {
	// 检查当前类
	if g.currentClassDecl != nil && symbol.ToGoName(g.currentClassDecl.Name, g.currentClassDecl.Public) == goClassName {
		return g.currentClassDecl.Name
	}
	// 检查当前结构体
	if g.currentStructDecl != nil && symbol.ToGoName(g.currentStructDecl.Name, g.currentStructDecl.Public) == goClassName {
		return g.currentStructDecl.Name
	}
	// 遍历类声明
	for _, classDecl := range g.transpiler.classDecls {
		if symbol.ToGoName(classDecl.Name, classDecl.Public) == goClassName {
			return classDecl.Name
		}
	}
	// 如果找不到，返回原名（可能是非公开类，名称相同）
	return goClassName
}

// getReceiverType 获取表达式的接收者类型（用于重载解析）
func (g *CodeGen) getReceiverType(expr parser.Expression) string {
	switch e := expr.(type) {
	case *parser.ThisExpr:
		// this 表达式，返回当前类/结构体名
		if g.currentClassDecl != nil {
			return g.currentClassDecl.Name
		}
		if g.currentStructDecl != nil {
			return g.currentStructDecl.Name
		}
		return ""
	case *parser.Identifier:
		// 首先检查跟踪的变量类型
		if typeName, ok := g.varTypes[e.Value]; ok {
			return typeName
		}
		// 检查是否是类名
		for _, classDecl := range g.transpiler.classDecls {
			if classDecl.Name == e.Value {
				return e.Value
			}
		}
		// 检查符号表中是否有此结构体
		if sym := g.transpiler.table.Get(g.transpiler.pkg, e.Value); sym != nil {
			if sym.Kind == symbol.SymbolStruct {
				return e.Value
			}
		}
		return ""
	case *parser.CallExpr:
		// 函数调用返回值，检查是否是 NewXxx() 模式
		if ident, ok := e.Function.(*parser.Identifier); ok {
			funcName := ident.Value
			if strings.HasPrefix(funcName, "New") && len(funcName) > 3 {
				typeName := funcName[3:]
				// 检查是否是已知的类
				for _, classDecl := range g.transpiler.classDecls {
					if classDecl.Name == typeName || strings.EqualFold(classDecl.Name, typeName) {
						return classDecl.Name
					}
				}
				// 检查符号表中是否有此结构体
				if sym := g.transpiler.table.Get(g.transpiler.pkg, typeName); sym != nil && sym.Kind == symbol.SymbolStruct {
					return typeName
				}
			}
		}
		return ""
	case *parser.NewExpr:
		// new 表达式，返回类名
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return ident.Value
		}
		return ""
	default:
		return ""
	}
}

// generateDefaultParamCall 生成带默认参数函数的调用
func (g *CodeGen) generateDefaultParamCall(sym *symbol.Symbol, args []parser.Expression) string {
	funcName := sym.GoName
	optsName := funcName + "__Opts"

	// 如果没有参数，使用默认值构造器
	if len(args) == 0 {
		return funcName + "(NewDefault__" + optsName + "())"
	}

	// 获取函数的参数信息
	funcDecl := g.transpiler.GetFuncDecl(sym.Package, sym.Name)
	if funcDecl == nil {
		// 回退到普通调用
		var argStrs []string
		for _, arg := range args {
			argStrs = append(argStrs, g.generateExpression(arg))
		}
		return funcName + "(" + strings.Join(argStrs, ", ") + ")"
	}

	// 构建 Opts 结构体
	var fields []string
	for i, param := range funcDecl.Params {
		fieldName := symbol.ToGoName(param.Name, true)
		var value string
		if i < len(args) {
			// 使用传入的参数
			value = g.generateExpression(args[i])
		} else if param.DefaultValue != nil {
			// 使用默认值
			value = g.generateExpression(param.DefaultValue)
		} else {
			continue
		}
		fields = append(fields, fieldName+": "+value)
	}

	return funcName + "(" + optsName + "{" + strings.Join(fields, ", ") + "})"
}

// generatePrintCall 生成 print 调用
func (g *CodeGen) generatePrintCall(funcName string, args []parser.Expression) string {
	var argStrs []string
	for _, arg := range args {
		argStrs = append(argStrs, g.generateExpression(arg))
	}
	return "fmt." + funcName + "(" + strings.Join(argStrs, ", ") + ")"
}

// generateIndexExpr 生成索引表达式
func (g *CodeGen) generateIndexExpr(expr *parser.IndexExpr) string {
	return g.generateExpression(expr.X) + "[" + g.generateExpression(expr.Index) + "]"
}

// generateSliceExpr 生成切片表达式
func (g *CodeGen) generateSliceExpr(expr *parser.SliceExpr) string {
	result := g.generateExpression(expr.X) + "["

	if expr.Low != nil {
		result += g.generateExpression(expr.Low)
	}
	result += ":"
	if expr.High != nil {
		result += g.generateExpression(expr.High)
	}
	if expr.Max != nil {
		result += ":" + g.generateExpression(expr.Max)
	}

	return result + "]"
}

// generateSelectorExpr 生成选择器表达式
func (g *CodeGen) generateSelectorExpr(expr *parser.SelectorExpr) string {
	x := g.generateExpression(expr.X)

	// 如果是 this.field/method（即接收者.成员），查找当前结构体/类的成员
	if _, ok := expr.X.(*parser.ThisExpr); ok {
		// 检查当前结构体的字段
		if g.currentStructDecl != nil {
			for _, field := range g.currentStructDecl.Fields {
				if field.Name == expr.Sel {
					fieldName := symbol.ToGoName(field.Name, field.Public)
					return x + "." + fieldName
				}
			}
			// 检查当前结构体的方法
			for _, method := range g.currentStructDecl.Methods {
				if method.Name == expr.Sel {
					isPublic := method.Visibility == "public" || method.Visibility == "protected"
					methodName := symbol.ToGoName(method.Name, isPublic)
					return x + "." + methodName
				}
			}
		}
		// 检查当前类的字段
		if g.currentClassDecl != nil {
			for _, field := range g.currentClassDecl.Fields {
				if field.Name == expr.Sel {
					isPublic := field.Visibility == "public" || field.Visibility == "protected"
					fieldName := symbol.ToGoName(field.Name, isPublic)
					return x + "." + fieldName
				}
			}
			// 检查当前类的方法
			for _, method := range g.currentClassDecl.Methods {
				if method.Name == expr.Sel {
					isPublic := method.Visibility == "public" || method.Visibility == "protected"
					methodName := symbol.ToGoName(method.Name, isPublic)
					return x + "." + methodName
				}
			}
		}
	}

	// 全局查找方法名（适用于 obj.method() 调用）
	// 遍历所有已知类和结构体，查找匹配的方法名
	methodGoName := g.transpiler.LookupMethodByName(expr.Sel)
	if methodGoName != "" {
		return x + "." + methodGoName
	}

	// 查找字段（可能需要转换大小写）
	sym := g.transpiler.LookupSymbol(expr.Sel)
	if sym != nil {
		return x + "." + sym.GoName
	}

	return x + "." + expr.Sel
}

// generateStaticAccessExpr 生成静态访问表达式 (ClassName::member 或 self::member)
func (g *CodeGen) generateStaticAccessExpr(expr *parser.StaticAccessExpr) string {
	var className string
	var classDecl *parser.ClassDecl
	var pkgPrefix string // 包前缀，用于导入的类

	// 获取类名和类声明
	switch left := expr.Left.(type) {
	case *parser.SelfExpr:
		// self:: 使用当前静态类
		className = g.currentReceiver
		classDecl = g.currentStaticClass
	case *parser.Identifier:
		// ClassName:: 直接使用类名
		className = left.Value
		// 检查是否是导入的类型（如标准库的 Str）
		if pkg, ok := g.typeToPackage[className]; ok {
			pkgPrefix = pkg + "."
		}
		// 从 transpiler 中查找静态类声明
		classDecl = g.transpiler.GetClassDecl(g.transpiler.pkg, className)
	default:
		return "/* invalid static access */"
	}

	memberName := expr.Member
	
	// 确定类是否是公开的
	isClassPublic := true
	if classDecl != nil {
		isClassPublic = classDecl.Public
	}
	goClassName := symbol.ToGoName(className, isClassPublic)

	// 特殊处理: ClassName::class 返回类信息
	if memberName == "class" {
		// 需要导入 tugo/lang 包
		g.tugoImports["tugo/lang"] = "lang"
		infoVarName := fmt.Sprintf("_%s_classInfo", goClassName)
		return pkgPrefix + infoVarName
	}

	// 在静态类中查找成员
	if classDecl != nil {
		// 检查是否是字段
		for _, field := range classDecl.Fields {
			if field.Name == memberName {
				isPublic := field.Visibility == "public"
				if isPublic {
					return pkgPrefix + goClassName + symbol.ToGoName(memberName, true)
				} else {
					return pkgPrefix + "_" + strings.ToLower(classDecl.Name) + "_" + memberName
				}
			}
		}

		// 检查是否是方法
		for _, method := range classDecl.Methods {
			if method.Name == memberName {
				isPublic := method.Visibility == "public"
				if isPublic {
					return pkgPrefix + goClassName + symbol.ToGoName(memberName, true)
				} else {
					return pkgPrefix + strings.ToLower(string(classDecl.Name[0])) + classDecl.Name[1:] + symbol.ToGoName(memberName, true)
				}
			}
		}
	}

	// 默认使用公开格式（带包前缀）
	return pkgPrefix + goClassName + symbol.ToGoName(memberName, true)
}

// generateTypeAssertExpr 生成类型断言表达式
func (g *CodeGen) generateTypeAssertExpr(expr *parser.TypeAssertExpr) string {
	return g.generateExpression(expr.X) + ".(" + g.generateType(expr.Type) + ")"
}

// generateArrayLiteral 生成数组字面量
func (g *CodeGen) generateArrayLiteral(lit *parser.ArrayLiteral) string {
	var elems []string
	for _, e := range lit.Elements {
		elems = append(elems, g.generateExpression(e))
	}
	return "[...]" + g.generateType(lit.Type) + "{" + strings.Join(elems, ", ") + "}"
}

// generateSliceLiteral 生成切片字面量
func (g *CodeGen) generateSliceLiteral(lit *parser.SliceLiteral) string {
	var elems []string
	for _, e := range lit.Elements {
		elems = append(elems, g.generateExpression(e))
	}
	return "[]" + g.generateType(lit.Type) + "{" + strings.Join(elems, ", ") + "}"
}

// generateMapLiteral 生成 map 字面量
func (g *CodeGen) generateMapLiteral(lit *parser.MapLiteral) string {
	var pairs []string
	for _, p := range lit.Pairs {
		pairs = append(pairs, g.generateExpression(p.Key)+": "+g.generateExpression(p.Value))
	}
	return "map[" + g.generateType(lit.KeyType) + "]" + g.generateType(lit.ValType) + "{" + strings.Join(pairs, ", ") + "}"
}

// generateStructLiteral 生成结构体字面量
func (g *CodeGen) generateStructLiteral(lit *parser.StructLiteral) string {
	typeExpr := g.generateExpression(lit.Type)
	var fields []string
	for _, f := range lit.Fields {
		if f.Name != "" {
			// 保持用户写的字段名不变
			// 用户应该使用与声明一致的名称（public 字段用大写，private 字段用小写）
			fields = append(fields, f.Name+": "+g.generateExpression(f.Value))
		} else {
			fields = append(fields, g.generateExpression(f.Value))
		}
	}
	return typeExpr + "{" + strings.Join(fields, ", ") + "}"
}

// generateFuncLiteral 生成函数字面量
func (g *CodeGen) generateFuncLiteral(lit *parser.FuncLiteral) string {
	var result strings.Builder
	result.WriteString("func(")

	// 参数
	for i, param := range lit.Params {
		if i > 0 {
			result.WriteString(", ")
		}
		if param.Name != "" {
			result.WriteString(symbol.TransformDollarVar(param.Name))
			result.WriteString(" ")
		}
		result.WriteString(g.generateType(param.Type))
	}
	result.WriteString(")")

	// 返回值
	if len(lit.Results) > 0 {
		result.WriteString(" ")
		if len(lit.Results) == 1 && lit.Results[0].Name == "" {
			result.WriteString(g.generateType(lit.Results[0].Type))
		} else {
			result.WriteString("(")
			for i, r := range lit.Results {
				if i > 0 {
					result.WriteString(", ")
				}
				if r.Name != "" {
					result.WriteString(symbol.TransformDollarVar(r.Name))
					result.WriteString(" ")
				}
				result.WriteString(g.generateType(r.Type))
			}
			result.WriteString(")")
		}
	}

	// 函数体
	result.WriteString(" {\n")
	for _, stmt := range lit.Body.Statements {
		// 简化处理，实际应该递归生成
		if exprStmt, ok := stmt.(*parser.ExpressionStmt); ok {
			result.WriteString("\t\t")
			result.WriteString(g.generateExpression(exprStmt.Expression))
			result.WriteString("\n")
		}
	}
	result.WriteString("\t}")

	return result.String()
}

// generateMakeExpr 生成 make 表达式
func (g *CodeGen) generateMakeExpr(expr *parser.MakeExpr) string {
	result := "make(" + g.generateType(expr.Type)
	for _, arg := range expr.Args {
		result += ", " + g.generateExpression(arg)
	}
	return result + ")"
}

// generateAppendExpr 生成 append 表达式
func (g *CodeGen) generateAppendExpr(expr *parser.AppendExpr) string {
	result := "append(" + g.generateExpression(expr.Slice)
	for _, e := range expr.Elems {
		result += ", " + g.generateExpression(e)
	}
	return result + ")"
}

// generateArrayType 生成数组类型
func (g *CodeGen) generateArrayType(t *parser.ArrayType) string {
	return "[" + g.generateExpression(t.Len) + "]" + g.generateType(t.Elt)
}

// generateChanType 生成通道类型
func (g *CodeGen) generateChanType(t *parser.ChanType) string {
	switch t.Dir {
	case 1:
		return "chan<- " + g.generateType(t.Value)
	case 2:
		return "<-chan " + g.generateType(t.Value)
	default:
		return "chan " + g.generateType(t.Value)
	}
}

// generateFuncType 生成函数类型
func (g *CodeGen) generateFuncType(t *parser.FuncType) string {
	var result strings.Builder
	result.WriteString("func(")

	for i, param := range t.Params {
		if i > 0 {
			result.WriteString(", ")
		}
		if param.Name != "" {
			result.WriteString(symbol.TransformDollarVar(param.Name))
			result.WriteString(" ")
		}
		result.WriteString(g.generateType(param.Type))
	}
	result.WriteString(")")

	if len(t.Results) > 0 {
		result.WriteString(" ")
		if len(t.Results) == 1 && t.Results[0].Name == "" {
			result.WriteString(g.generateType(t.Results[0].Type))
		} else {
			result.WriteString("(")
			for i, r := range t.Results {
				if i > 0 {
					result.WriteString(", ")
				}
				if r.Name != "" {
					result.WriteString(symbol.TransformDollarVar(r.Name))
					result.WriteString(" ")
				}
				result.WriteString(g.generateType(r.Type))
			}
			result.WriteString(")")
		}
	}

	return result.String()
}

// generateInterfaceType 生成接口类型
func (g *CodeGen) generateInterfaceType(t *parser.InterfaceType) string {
	if len(t.Methods) == 0 {
		return "interface{}"
	}

	var result strings.Builder
	result.WriteString("interface {\n")
	for _, m := range t.Methods {
		result.WriteString("\t\t")
		result.WriteString(symbol.ToGoName(m.Name, true))
		result.WriteString("(")
		for i, param := range m.Params {
			if i > 0 {
				result.WriteString(", ")
			}
			if param.Name != "" {
				result.WriteString(symbol.TransformDollarVar(param.Name))
				result.WriteString(" ")
			}
			result.WriteString(g.generateType(param.Type))
		}
		result.WriteString(")")
		if len(m.Results) > 0 {
			result.WriteString(" ")
			if len(m.Results) == 1 && m.Results[0].Name == "" {
				result.WriteString(g.generateType(m.Results[0].Type))
			} else {
				result.WriteString("(")
				for i, r := range m.Results {
					if i > 0 {
						result.WriteString(", ")
					}
					if r.Name != "" {
						result.WriteString(symbol.TransformDollarVar(r.Name))
						result.WriteString(" ")
					}
					result.WriteString(g.generateType(r.Type))
				}
				result.WriteString(")")
			}
		}
		result.WriteString("\n")
	}
	result.WriteString("\t}")
	return result.String()
}

// generateStructType 生成结构体类型
func (g *CodeGen) generateStructType(t *parser.StructType) string {
	if len(t.Fields) == 0 {
		return "struct{}"
	}

	var result strings.Builder
	result.WriteString("struct {\n")
	for _, f := range t.Fields {
		result.WriteString("\t\t")
		result.WriteString(symbol.ToGoName(f.Name, f.Public))
		result.WriteString(" ")
		result.WriteString(g.generateType(f.Type))
		if f.Tag != "" {
			result.WriteString(" ")
			result.WriteString(f.Tag)
		}
		result.WriteString("\n")
	}
	result.WriteString("\t}")
	return result.String()
}

// generateNewExpr 生成 new 表达式
// 支持两种情况:
// 1. Go 风格: new(Type) -> new(Type)
// 2. OOP 风格: new ClassName() -> NewClassName(NewDefaultClassNameInitOpts())
//             new ClassName(name: "test") -> NewClassName(ClassNameInitOpts{Name: "test"})
func (g *CodeGen) generateNewExpr(expr *parser.NewExpr) string {
	// 获取类名
	className := ""
	if ident, ok := expr.Type.(*parser.Identifier); ok {
		className = ident.Value
	} else {
		// Go 风格的 new(Type)
		return "new(" + g.generateType(expr.Type) + ")"
	}

	// OOP 风格的 new ClassName(args)
	// 检查是否是导入的类型
	pkgPrefix := ""
	if pkg, ok := g.typeToPackage[className]; ok {
		pkgPrefix = pkg + "."
	}

	// 查找类声明以确定是否公开和是否有 init 参数
	classDecl := g.transpiler.GetClassDecl(g.transpiler.pkg, className)
	isClassPublic := true
	hasInitParams := false  // 是否有 init 参数（需要 opts 模式）
	if classDecl != nil {
		isClassPublic = classDecl.Public
		// 检查是否有 init 方法且有参数
		if classDecl.InitMethod != nil && len(classDecl.InitMethod.Params) > 0 {
			hasInitParams = true
		}
	} else {
		// 外部包的类，无法获取类声明，保持原有行为（使用 opts 模式）
		// 这是更安全的默认行为，因为大多数类都有构造参数
		hasInitParams = true
	}
	goClassName := symbol.ToGoName(className, isClassPublic)

	// 生成构造函数调用
	if len(expr.Arguments) == 0 {
		if hasInitParams {
			// 有 init 参数: New__ClassName(NewDefault__ClassName__InitOpts())
			return fmt.Sprintf("%sNew__%s(%sNewDefault__%s__InitOpts())", pkgPrefix, goClassName, pkgPrefix, goClassName)
		}
		// 无 init 参数: New__ClassName()
		return fmt.Sprintf("%sNew__%s()", pkgPrefix, goClassName)
	}

	// 有参数: 生成 opts 结构体
	var argStrs []string
	for _, arg := range expr.Arguments {
		argStrs = append(argStrs, g.generateExpression(arg))
	}

	// 生成: New__ClassName(ClassName__InitOpts{args...})
	return fmt.Sprintf("%sNew__%s(%s%s__InitOpts{%s})", pkgPrefix, goClassName, pkgPrefix, goClassName, strings.Join(argStrs, ", "))
}

// generateType 生成类型
func (g *CodeGen) generateType(expr parser.Expression) string {
	return g.generateExpression(expr)
}

// inferExprType 推断表达式的类型（用于重载解析）
func (g *CodeGen) inferExprType(expr parser.Expression) string {
	if expr == nil {
		return "any"
	}

	switch e := expr.(type) {
	case *parser.IntegerLiteral:
		return "int"
	case *parser.FloatLiteral:
		return "float64"
	case *parser.StringLiteral:
		return "string"
	case *parser.CharLiteral:
		return "rune"
	case *parser.BoolLiteral:
		return "bool"
	case *parser.NilLiteral:
		return "nil"
	case *parser.Identifier:
		// 查找变量类型（简化：返回 any）
		// 实际实现需要维护变量类型表
		return "any"
	case *parser.UnaryExpr:
		if e.Operator == "&" {
			return "p" + g.inferExprType(e.Operand)
		} else if e.Operator == "*" {
			inner := g.inferExprType(e.Operand)
			if len(inner) > 1 && inner[0] == 'p' {
				return inner[1:]
			}
			return "any"
		}
		return g.inferExprType(e.Operand)
	case *parser.CallExpr:
		// 函数调用返回类型（简化：返回 any）
		return "any"
	case *parser.ArrayLiteral:
		if e.Type != nil {
			return "s" + symbol.GenerateTypeSignature(e.Type)
		}
		return "sany"
	case *parser.SliceLiteral:
		if e.Type != nil {
			return "s" + symbol.GenerateTypeSignature(e.Type)
		}
		return "sany"
	case *parser.MapLiteral:
		if e.KeyType != nil && e.ValType != nil {
			return "m" + symbol.GenerateTypeSignature(e.KeyType) + symbol.GenerateTypeSignature(e.ValType)
		}
		return "manyany"
	case *parser.StructLiteral:
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return ident.Value
		}
		return "any"
	case *parser.NewExpr:
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return "p" + ident.Value
		}
		return "pany"
	case *parser.BinaryExpr:
		// 二元运算返回与操作数相同类型
		return g.inferExprType(e.Left)
	case *parser.IndexExpr:
		// 索引表达式（简化）
		return "any"
	case *parser.SelectorExpr:
		// 选择器表达式（简化）
		return "any"
	case *parser.ParenExpr:
		return g.inferExprType(e.X)
	default:
		return "any"
	}
}

// getClassPackage 获取类所在的包名
func (g *CodeGen) getClassPackage(className string) string {
	// 首先检查当前包
	if g.transpiler.table.GetClass(g.transpiler.pkg, className) != nil {
		return g.transpiler.pkg
	}
	// 检查导入的包
	if pkgName, ok := g.typeToPackage[className]; ok {
		return pkgName
	}
	// 默认返回当前包
	return g.transpiler.pkg
}

// resolveOverloadedMethod 解析重载方法调用，返回修饰后的方法名
func (g *CodeGen) resolveOverloadedMethod(receiver, methodName string, args []parser.Expression) string {
	return g.resolveOverloadedMethodInPkg(g.transpiler.pkg, receiver, methodName, args)
}

// resolveOverloadedMethodInPkg 在指定包中解析重载方法调用
func (g *CodeGen) resolveOverloadedMethodInPkg(pkg, receiver, methodName string, args []parser.Expression) string {
	mangledName, _ := g.resolveOverloadedMethodWithInfo(pkg, receiver, methodName, args)
	return mangledName
}

// resolveOverloadedMethodWithInfo 在指定包中解析重载方法调用，同时返回方法信息
func (g *CodeGen) resolveOverloadedMethodWithInfo(pkg, receiver, methodName string, args []parser.Expression) (string, *symbol.OverloadedMethod) {
	// 推断参数类型
	var argTypes []string
	for _, arg := range args {
		argTypes = append(argTypes, g.inferExprType(arg))
	}

	// 查找匹配的重载
	group := g.transpiler.table.GetOverloadGroup(pkg, receiver, methodName)
	if group != nil {
		// 首先尝试精确匹配
		for _, method := range group.Methods {
			if len(method.ParamTypes) == len(argTypes) {
				match := true
				for i, pt := range method.ParamTypes {
					if pt != argTypes[i] && argTypes[i] != "any" {
						match = false
						break
					}
				}
				if match {
					return method.MangledName, method
				}
			}
		}
		
		// 如果精确匹配失败，尝试按参数数量匹配
		for _, method := range group.Methods {
			if len(method.ParamTypes) == len(args) {
				return method.MangledName, method
			}
		}
		
		// 如果还是没找到，返回第一个版本
		if len(group.Methods) > 0 {
			return group.Methods[0].MangledName, group.Methods[0]
		}
	}

	// 回退到标准名称转换
	return symbol.ToGoName(methodName, true), nil
}

// generateOptsCall 生成使用 Opts 结构体的方法调用
func (g *CodeGen) generateOptsCall(receiver, classPkg, className, mangledName string, method *symbol.OverloadedMethod, args []parser.Expression) string {
	// 获取 Go 类名（用于 Opts 结构体名）
	classInfo := g.transpiler.table.GetClass(classPkg, className)
	goClassName := className
	if classInfo != nil {
		goClassName = classInfo.GoName
	}
	
	optsName := goClassName + "__" + mangledName + "__Opts"
	
	// 检查是否需要包前缀
	pkgPrefix := ""
	if classPkg != g.transpiler.pkg {
		// 跨包调用，需要添加包前缀
		// 从 typeToPackage 获取导入别名
		for typeName, pkgName := range g.typeToPackage {
			if typeName == className {
				pkgPrefix = pkgName + "."
				break
			}
		}
	}
	
	// 生成 Opts 结构体字面量
	var fields []string
	for i, arg := range args {
		if i < len(method.ParamNames) {
			fieldName := symbol.ToGoName(method.ParamNames[i], true)
			argVal := g.generateExpression(arg)
			fields = append(fields, fmt.Sprintf("%s: %s", fieldName, argVal))
		}
	}
	
	optsLiteral := pkgPrefix + optsName + "{" + strings.Join(fields, ", ") + "}"
	return receiver + "." + mangledName + "(" + optsLiteral + ")"
}

// write 写入内容
func (g *CodeGen) write(s string) {
	g.builder.WriteString(s)
}

// writeLine 写入一行
func (g *CodeGen) writeLine(s string) {
	g.writeIndent()
	g.builder.WriteString(s)
	g.builder.WriteString("\n")
}

// writeIndent 写入缩进
func (g *CodeGen) writeIndent() {
	for i := 0; i < g.indent; i++ {
		g.builder.WriteString("\t")
	}
}
