package transpiler

import (
	"fmt"
	"sort"
	"strings"

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
}

// NewCodeGen 创建一个新的代码生成器
func NewCodeGen(t *Transpiler) *CodeGen {
	return &CodeGen{
		transpiler:    t,
		typeToPackage: make(map[string]string),
		goImports:     make(map[string]bool),
		tugoImports:   make(map[string]string),
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

		// 扫描类的方法
		if classDecl, ok := stmt.(*parser.ClassDecl); ok {
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
			case "print", "println", "print_f":
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
		g.writeLine(g.generateExpression(s.Expression))
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
		if len(decl.Results) == 1 && decl.Results[0].Name == "" {
			g.write(g.generateType(decl.Results[0].Type))
		} else {
			g.write("(")
			g.generateParams(decl.Results)
			g.write(")")
		}
	}

	// 函数体
	if decl.Body != nil {
		g.write(" ")
		g.generateBlockStmtInline(decl.Body)
	}
	g.writeLine("")
}

// generateFuncWithDefaults 生成带默认参数的函数
func (g *CodeGen) generateFuncWithDefaults(decl *parser.FuncDecl) {
	funcName := symbol.ToGoName(decl.Name, decl.Public)
	optsName := funcName + "Opts"

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
	g.writeLine(fmt.Sprintf("func NewDefault%s() %s {", optsName, optsName))
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
		optsName := structName + "InitOpts"
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
		g.writeLine(fmt.Sprintf("func NewDefault%s() %s {", optsName, optsName))
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
		g.writeLine(fmt.Sprintf("func New%s(opts %s) *%s {", structName, optsName, structName))
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
		g.write(fmt.Sprintf("func New%s(", structName))
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
	methodName := symbol.ToGoName(method.Name, isPublic)

	// 方法签名（指针接收者）
	g.write(fmt.Sprintf("func (s *%s) %s(", structName, methodName))
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

	// 方法体（翻译 this 为 s）
	g.currentReceiver = "s"
	g.currentStructDecl = decl
	g.generateBlockStmt(method.Body)
	g.currentReceiver = ""
	g.currentStructDecl = nil
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
	// 命名规则: 公开方法 -> ClassName + MethodName, 私有方法 -> className + MethodName (首字母小写)
	var funcName string
	if isPublic {
		funcName = className + symbol.ToGoName(method.Name, true)
	} else {
		funcName = strings.ToLower(string(decl.Name[0])) + decl.Name[1:] + symbol.ToGoName(method.Name, true)
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

	// 5. 入口类：生成 Go 的 func main()
	if isEntryClass && mainMethod != nil {
		g.generateGoMainFunc(decl, mainMethod)
		g.writeLine("")
	}

	// 清理当前类上下文
	g.currentClassDecl = nil
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
	g.write(fmt.Sprintf("func New%s(", className))

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
	optsName := fmt.Sprintf("%sInitOpts", className)

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
	g.writeLine(fmt.Sprintf("func NewDefault%s() %s {", optsName, optsName))
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
	g.writeLine(fmt.Sprintf("func New%s(opts %s) *%s {", className, optsName, className))
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
	g.writeLine(fmt.Sprintf("func New%s() *%s {", className, className))
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
	g.write(fmt.Sprintf("func New%s(", className))

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
	optsName := fmt.Sprintf("%sInitOpts", className)

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
	g.writeLine(fmt.Sprintf("func NewDefault%s() %s {", optsName, optsName))
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
	g.writeLine(fmt.Sprintf("func New%s(opts %s) *%s {", className, optsName, className))
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
	g.writeLine(fmt.Sprintf("func New%s() *%s {", className, className))
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

// generateClassMethodSimple 生成简单方法
func (g *CodeGen) generateClassMethodSimple(className, methodName string, method *parser.ClassMethod) {
	g.writeIndent()
	g.write(fmt.Sprintf("func (t *%s) %s(", className, methodName))

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
		if len(method.Results) == 1 && method.Results[0].Name == "" {
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
	}

	g.writeLine(" {")
	g.indent++

	// 方法体
	if method.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range method.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
	}

	g.indent--
	g.writeLine("}")
}

// generateClassMethodWithDefaults 生成带默认参数的方法
func (g *CodeGen) generateClassMethodWithDefaults(className, methodName string, method *parser.ClassMethod) {
	optsName := fmt.Sprintf("%s%sOpts", className, methodName)

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
	g.writeLine(fmt.Sprintf("func NewDefault%s() %s {", optsName, optsName))
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
	g.write(fmt.Sprintf("func (t *%s) %s(opts %s)", className, methodName, optsName))

	// 返回值
	if len(method.Results) > 0 {
		g.write(" ")
		if len(method.Results) == 1 && method.Results[0].Name == "" {
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
	}

	g.writeLine(" {")
	g.indent++

	// 从 opts 获取参数
	for _, param := range method.Params {
		varName := symbol.TransformDollarVar(param.Name)
		fieldName := symbol.ToGoName(param.Name, true)
		g.writeLine(fmt.Sprintf("%s := opts.%s", varName, fieldName))
	}

	// 方法体
	if method.Body != nil {
		g.currentReceiver = "t"
		for _, stmt := range method.Body.Statements {
			g.generateStatement(stmt)
		}
		g.currentReceiver = ""
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

	g.writeLine(strings.Join(names, ", ") + " := " + g.generateExpression(decl.Value))
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
		g.writeLine("return")
		return
	}

	var values []string
	for _, v := range stmt.Values {
		values = append(values, g.generateExpression(v))
	}
	g.writeLine("return " + strings.Join(values, ", "))
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
		}

		// 检查是否是带默认参数的函数
		sym := g.transpiler.LookupSymbol(ident.Value)
		if sym != nil && sym.HasDefault {
			return g.generateDefaultParamCall(sym, expr.Arguments)
		}
	}

	funcExpr := g.generateExpression(expr.Function)
	var args []string
	for _, arg := range expr.Arguments {
		args = append(args, g.generateExpression(arg))
	}
	return funcExpr + "(" + strings.Join(args, ", ") + ")"
}

// generateDefaultParamCall 生成带默认参数函数的调用
func (g *CodeGen) generateDefaultParamCall(sym *symbol.Symbol, args []parser.Expression) string {
	funcName := sym.GoName
	optsName := funcName + "Opts"

	// 如果没有参数，使用默认值构造器
	if len(args) == 0 {
		return funcName + "(NewDefault" + optsName + "())"
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
	goClassName := symbol.ToGoName(className, true)

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

	goClassName := symbol.ToGoName(className, true)

	// 生成构造函数调用
	if len(expr.Arguments) == 0 {
		// 无参数: NewClassName(NewDefaultClassNameInitOpts())
		return fmt.Sprintf("%sNew%s(%sNewDefault%sInitOpts())", pkgPrefix, goClassName, pkgPrefix, goClassName)
	}

	// 有参数: 生成 opts 结构体
	var argStrs []string
	for _, arg := range expr.Arguments {
		argStrs = append(argStrs, g.generateExpression(arg))
	}

	// 生成: NewClassName(ClassNameInitOpts{args...})
	return fmt.Sprintf("%sNew%s(%s%sInitOpts{%s})", pkgPrefix, goClassName, pkgPrefix, goClassName, strings.Join(argStrs, ", "))
}

// generateType 生成类型
func (g *CodeGen) generateType(expr parser.Expression) string {
	return g.generateExpression(expr)
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
