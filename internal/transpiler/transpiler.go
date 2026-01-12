package transpiler

import (
	"fmt"

	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
)

// Transpiler 转译器
type Transpiler struct {
	table          *symbol.Table
	pkg            string
	imports        map[string]bool                    // 需要导入的包
	needFmt        bool                               // 是否需要导入 fmt
	funcDecls      map[string]*parser.FuncDecl        // 函数声明缓存 key: pkg.name
	classDecls     map[string]*parser.ClassDecl       // 类声明缓存 key: pkg.name
	interfaceDecls map[string]*parser.InterfaceDecl   // 接口声明缓存 key: pkg.name
	errors         []string                           // 转译错误
	config         *config.Config                     // 项目配置
	typeImports    map[string]string                  // 类型名到包名的映射 (User -> models)
	currentFile    string                             // 当前文件名（不含路径和后缀）
}

// New 创建一个新的转译器
func New(table *symbol.Table) *Transpiler {
	return &Transpiler{
		table:          table,
		imports:        make(map[string]bool),
		funcDecls:      make(map[string]*parser.FuncDecl),
		classDecls:     make(map[string]*parser.ClassDecl),
		interfaceDecls: make(map[string]*parser.InterfaceDecl),
		errors:         []string{},
		typeImports:    make(map[string]string),
	}
}

// SetConfig 设置项目配置
func (t *Transpiler) SetConfig(cfg *config.Config) {
	t.config = cfg
}

// GetConfig 获取项目配置
func (t *Transpiler) GetConfig() *config.Config {
	if t.config == nil {
		return config.DefaultConfig()
	}
	return t.config
}

// TranspileFile 转译单个文件（无文件名）
func (t *Transpiler) TranspileFile(file *parser.File) (string, error) {
	return t.TranspileFileWithName(file, "")
}

// TranspileFileWithName 转译单个文件（带文件名，用于验证）
func (t *Transpiler) TranspileFileWithName(file *parser.File, fileName string) (string, error) {
	t.pkg = file.Package
	t.imports = make(map[string]bool)
	t.needFmt = false
	t.errors = []string{}
	t.currentFile = fileName

	// 缓存声明
	for _, stmt := range file.Statements {
		switch decl := stmt.(type) {
		case *parser.FuncDecl:
			key := t.pkg + "." + decl.Name
			t.funcDecls[key] = decl
		case *parser.ClassDecl:
			key := t.pkg + "." + decl.Name
			t.classDecls[key] = decl
		case *parser.InterfaceDecl:
			key := t.pkg + "." + decl.Name
			t.interfaceDecls[key] = decl
		}
	}

	// 验证顶层语句（禁止类外的 func/const/var）
	t.validateTopLevelStatements(file)

	// 验证文件命名规则（public class/interface 必须与文件名一致）
	if fileName != "" {
		t.validateFileNaming(file, fileName)
	}

	// 校验 implements、extends 和 static class
	for _, stmt := range file.Statements {
		if classDecl, ok := stmt.(*parser.ClassDecl); ok {
			if classDecl.Static {
				t.validateStaticClass(classDecl)
			}
			if len(classDecl.Implements) > 0 {
				t.validateImplements(classDecl)
			}
			if classDecl.Extends != "" {
				t.validateExtends(classDecl)
			}
			// 验证入口类的 main 方法
			if t.IsEntryClass(classDecl) {
				t.validateMainMethod(classDecl)
			}
		}
		// 校验结构体
		if structDecl, ok := stmt.(*parser.StructDecl); ok {
			if len(structDecl.Implements) > 0 {
				t.validateStructImplements(structDecl)
			}
		}
	}

	// 如果有错误，返回错误
	if len(t.errors) > 0 {
		return "", &ImplementsError{Errors: t.errors}
	}

	gen := NewCodeGen(t)
	return gen.Generate(file), nil
}

// GetCurrentFile 获取当前文件名
func (t *Transpiler) GetCurrentFile() string {
	return t.currentFile
}

// TranspileFiles 转译多个文件（同一个包）
func (t *Transpiler) TranspileFiles(files []*parser.File) (map[string]string, error) {
	result := make(map[string]string)
	for i, file := range files {
		// 假设文件名根据索引生成（实际应该传入文件名）
		name := file.Package
		if i > 0 {
			name = file.Package + "_" + string(rune('0'+i))
		}
		code, err := t.TranspileFile(file)
		if err != nil {
			return nil, err
		}
		result[name] = code
	}
	return result, nil
}

// LookupSymbol 查找符号
func (t *Transpiler) LookupSymbol(name string) *symbol.Symbol {
	return t.table.Get(t.pkg, name)
}

// LookupMethod 查找方法
func (t *Transpiler) LookupMethod(receiver, name string) *symbol.Symbol {
	return t.table.GetMethod(t.pkg, receiver, name)
}

// LookupMethodByName 根据方法名查找方法的 Go 名称
// 遍历所有已知类和结构体，查找匹配的方法名
func (t *Transpiler) LookupMethodByName(name string) string {
	// 查找类的方法
	for _, classDecl := range t.classDecls {
		for _, method := range classDecl.Methods {
			if method.Name == name {
				isPublic := method.Visibility == "public" || method.Visibility == "protected"
				return symbol.ToGoName(method.Name, isPublic)
			}
		}
	}

	// 查找符号表中的方法
	sym := t.table.GetMethodByName(t.pkg, name)
	if sym != nil {
		return sym.GoName
	}

	return ""
}

// GetFuncDecl 获取函数声明
func (t *Transpiler) GetFuncDecl(pkg, name string) *parser.FuncDecl {
	key := pkg + "." + name
	return t.funcDecls[key]
}

// GetClassDecl 获取类声明
func (t *Transpiler) GetClassDecl(pkg, name string) *parser.ClassDecl {
	key := pkg + "." + name
	return t.classDecls[key]
}

// AddImport 添加需要导入的包
func (t *Transpiler) AddImport(pkg string) {
	t.imports[pkg] = true
}

// GetImports 获取需要导入的包
func (t *Transpiler) GetImports() []string {
	var result []string
	for pkg := range t.imports {
		result = append(result, pkg)
	}
	return result
}

// SetNeedFmt 设置是否需要导入 fmt
func (t *Transpiler) SetNeedFmt(need bool) {
	t.needFmt = need
	if need {
		t.imports["fmt"] = true
	}
}

// NeedFmt 是否需要导入 fmt
func (t *Transpiler) NeedFmt() bool {
	return t.needFmt
}

// Transpile 转译源代码
func Transpile(source string) (string, error) {
	// 解析
	file, errors := parser.Parse(source)
	if len(errors) > 0 {
		return "", &ParseError{Errors: errors}
	}

	// 收集符号
	table := symbol.Collect([]*parser.File{file})

	// 转译
	t := New(table)
	return t.TranspileFile(file)
}

// TranspileWithTable 使用已有符号表转译
func TranspileWithTable(source string, table *symbol.Table) (string, error) {
	// 解析
	file, errors := parser.Parse(source)
	if len(errors) > 0 {
		return "", &ParseError{Errors: errors}
	}

	// 转译
	t := New(table)
	return t.TranspileFile(file)
}

// ParseError 解析错误
type ParseError struct {
	Errors []string
}

func (e *ParseError) Error() string {
	if len(e.Errors) == 0 {
		return "parse error"
	}
	return e.Errors[0]
}

// ImplementsError 接口实现错误
type ImplementsError struct {
	Errors []string
}

func (e *ImplementsError) Error() string {
	if len(e.Errors) == 0 {
		return "implements error"
	}
	return e.Errors[0]
}

// validateImplements 校验类是否正确实现了所有声明的接口
func (t *Transpiler) validateImplements(classDecl *parser.ClassDecl) {
	// 收集类的所有方法签名
	classMethods := make(map[string]*parser.ClassMethod)
	for _, method := range classDecl.Methods {
		classMethods[method.Name] = method
	}

	// 遍历每个需要实现的接口
	for _, ifaceName := range classDecl.Implements {
		// 查找接口定义
		ifaceInfo := t.table.GetInterface(t.pkg, ifaceName)
		if ifaceInfo == nil {
			t.errors = append(t.errors, fmt.Sprintf(
				"class %s: interface %s not found",
				classDecl.Name, ifaceName))
			continue
		}

		// 检查接口的每个方法
		for _, ifaceMethod := range ifaceInfo.Methods {
			classMethod, exists := classMethods[ifaceMethod.Name]
			if !exists {
				t.errors = append(t.errors, fmt.Sprintf(
					"class %s does not implement interface %s: missing method %s",
					classDecl.Name, ifaceName, ifaceMethod.Name))
				continue
			}

			// 校验方法签名
			if err := t.validateMethodSignature(classDecl.Name, ifaceName, classMethod, ifaceMethod); err != "" {
				t.errors = append(t.errors, err)
			}
		}
	}
}

// validateMethodSignature 校验方法签名是否匹配
func (t *Transpiler) validateMethodSignature(className, ifaceName string, method *parser.ClassMethod, sig *parser.FuncSignature) string {
	// 检查参数数量
	if len(method.Params) != len(sig.Params) {
		return fmt.Sprintf(
			"class %s method %s: parameter count mismatch (got %d, interface %s requires %d)",
			className, method.Name, len(method.Params), ifaceName, len(sig.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(sig.Results) {
		return fmt.Sprintf(
			"class %s method %s: return value count mismatch (got %d, interface %s requires %d)",
			className, method.Name, len(method.Results), ifaceName, len(sig.Results))
	}

	// 注意：类型匹配校验需要更复杂的类型比较，这里做简化处理
	// 实际上应该比较每个参数和返回值的类型是否一致

	return ""
}

// validateStructImplements 校验结构体是否正确实现了接口
func (t *Transpiler) validateStructImplements(structDecl *parser.StructDecl) {
	// 收集结构体的所有方法签名
	structMethods := make(map[string]*parser.ClassMethod)
	for _, method := range structDecl.Methods {
		structMethods[method.Name] = method
	}

	// 遍历每个需要实现的接口
	for _, ifaceName := range structDecl.Implements {
		// 查找接口定义
		ifaceInfo := t.table.GetInterface(t.pkg, ifaceName)
		if ifaceInfo == nil {
			t.errors = append(t.errors, fmt.Sprintf(
				"struct %s: interface %s not found",
				structDecl.Name, ifaceName))
			continue
		}

		// 检查接口的每个方法
		for _, ifaceMethod := range ifaceInfo.Methods {
			structMethod, exists := structMethods[ifaceMethod.Name]
			if !exists {
				t.errors = append(t.errors, fmt.Sprintf(
					"struct %s does not implement interface %s: missing method %s",
					structDecl.Name, ifaceName, ifaceMethod.Name))
				continue
			}

			// 校验方法签名
			if err := t.validateStructMethodSignature(structDecl.Name, ifaceName, structMethod, ifaceMethod); err != "" {
				t.errors = append(t.errors, err)
			}
		}
	}
}

// validateStructMethodSignature 校验结构体方法签名是否匹配
func (t *Transpiler) validateStructMethodSignature(structName, ifaceName string, method *parser.ClassMethod, sig *parser.FuncSignature) string {
	// 检查参数数量
	if len(method.Params) != len(sig.Params) {
		return fmt.Sprintf(
			"struct %s method %s: parameter count mismatch (got %d, interface %s requires %d)",
			structName, method.Name, len(method.Params), ifaceName, len(sig.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(sig.Results) {
		return fmt.Sprintf(
			"struct %s method %s: return value count mismatch (got %d, interface %s requires %d)",
			structName, method.Name, len(method.Results), ifaceName, len(sig.Results))
	}

	return ""
}

// validateExtends 校验子类是否正确实现了父类的所有抽象方法
func (t *Transpiler) validateExtends(classDecl *parser.ClassDecl) {
	// 查找父类信息
	parentInfo := t.table.GetClass(t.pkg, classDecl.Extends)
	if parentInfo == nil {
		t.errors = append(t.errors, fmt.Sprintf(
			"class %s: parent class %s not found",
			classDecl.Name, classDecl.Extends))
		return
	}

	// 父类必须是抽象类
	if !parentInfo.Abstract {
		t.errors = append(t.errors, fmt.Sprintf(
			"class %s: cannot extend non-abstract class %s",
			classDecl.Name, classDecl.Extends))
		return
	}

	// 如果子类也是抽象类，不需要实现父类的抽象方法
	if classDecl.Abstract {
		return
	}

	// 收集子类的所有方法
	childMethods := make(map[string]*parser.ClassMethod)
	for _, method := range classDecl.Methods {
		childMethods[method.Name] = method
	}

	// 检查是否实现了父类的所有抽象方法
	for _, abstractMethod := range parentInfo.AbstractMethods {
		childMethod, exists := childMethods[abstractMethod.Name]
		if !exists {
			t.errors = append(t.errors, fmt.Sprintf(
				"class %s does not implement abstract method %s from parent class %s",
				classDecl.Name, abstractMethod.Name, classDecl.Extends))
			continue
		}

		// 校验方法签名
		if err := t.validateAbstractMethodSignature(classDecl.Name, classDecl.Extends, childMethod, abstractMethod); err != "" {
			t.errors = append(t.errors, err)
		}
	}
}

// validateAbstractMethodSignature 校验抽象方法签名是否匹配
func (t *Transpiler) validateAbstractMethodSignature(className, parentName string, method *parser.ClassMethod, abstractMethod *parser.ClassMethod) string {
	// 检查参数数量
	if len(method.Params) != len(abstractMethod.Params) {
		return fmt.Sprintf(
			"class %s method %s: parameter count mismatch (got %d, abstract method in %s requires %d)",
			className, method.Name, len(method.Params), parentName, len(abstractMethod.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(abstractMethod.Results) {
		return fmt.Sprintf(
			"class %s method %s: return value count mismatch (got %d, abstract method in %s requires %d)",
			className, method.Name, len(method.Results), parentName, len(abstractMethod.Results))
	}

	return ""
}

// validateStaticClass 校验静态类
func (t *Transpiler) validateStaticClass(classDecl *parser.ClassDecl) {
	// 静态类不能有 init 构造函数
	if classDecl.InitMethod != nil {
		t.errors = append(t.errors, fmt.Sprintf(
			"static class %s cannot have init constructor",
			classDecl.Name))
	}

	// 静态类方法体内不能使用 this
	for _, method := range classDecl.Methods {
		if method.Body != nil {
			if t.containsThis(method.Body) {
				t.errors = append(t.errors, fmt.Sprintf(
					"static class %s method %s cannot use 'this', use 'self::' instead",
					classDecl.Name, method.Name))
			}
		}
	}
}

// containsThis 检查代码块是否包含 this
func (t *Transpiler) containsThis(block *parser.BlockStmt) bool {
	for _, stmt := range block.Statements {
		if t.stmtContainsThis(stmt) {
			return true
		}
	}
	return false
}

// stmtContainsThis 检查语句是否包含 this
func (t *Transpiler) stmtContainsThis(stmt parser.Statement) bool {
	switch s := stmt.(type) {
	case *parser.ExpressionStmt:
		return t.exprContainsThis(s.Expression)
	case *parser.ReturnStmt:
		for _, v := range s.Values {
			if t.exprContainsThis(v) {
				return true
			}
		}
	case *parser.AssignStmt:
		for _, l := range s.Left {
			if t.exprContainsThis(l) {
				return true
			}
		}
		for _, r := range s.Right {
			if t.exprContainsThis(r) {
				return true
			}
		}
	case *parser.IfStmt:
		if t.exprContainsThis(s.Condition) {
			return true
		}
		if s.Consequence != nil && t.containsThis(s.Consequence) {
			return true
		}
		if alt, ok := s.Alternative.(*parser.BlockStmt); ok {
			if t.containsThis(alt) {
				return true
			}
		}
	case *parser.ForStmt:
		if s.Body != nil && t.containsThis(s.Body) {
			return true
		}
	case *parser.BlockStmt:
		return t.containsThis(s)
	}
	return false
}

// exprContainsThis 检查表达式是否包含 this
func (t *Transpiler) exprContainsThis(expr parser.Expression) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *parser.ThisExpr:
		return true
	case *parser.BinaryExpr:
		return t.exprContainsThis(e.Left) || t.exprContainsThis(e.Right)
	case *parser.UnaryExpr:
		return t.exprContainsThis(e.Operand)
	case *parser.CallExpr:
		if t.exprContainsThis(e.Function) {
			return true
		}
		for _, arg := range e.Arguments {
			if t.exprContainsThis(arg) {
				return true
			}
		}
	case *parser.SelectorExpr:
		return t.exprContainsThis(e.X)
	case *parser.IndexExpr:
		return t.exprContainsThis(e.X) || t.exprContainsThis(e.Index)
	}
	return false
}

// validateTopLevelStatements 验证顶层语句
// 只允许 interface、struct、class 在顶层
// 禁止类外的 func、const、var
func (t *Transpiler) validateTopLevelStatements(file *parser.File) {
	for _, stmt := range file.Statements {
		switch s := stmt.(type) {
		case *parser.ClassDecl:
			// 允许
		case *parser.StructDecl:
			// 允许
		case *parser.InterfaceDecl:
			// 允许
		case *parser.TypeDecl:
			// 允许类型别名
		case *parser.FuncDecl:
			t.errors = append(t.errors, fmt.Sprintf(
				"functions must be defined inside a class, found top-level function '%s'", s.Name))
		case *parser.VarDecl:
			names := ""
			if len(s.Names) > 0 {
				names = s.Names[0]
			}
			t.errors = append(t.errors, fmt.Sprintf(
				"variables must be defined inside a class, found top-level variable '%s'", names))
		case *parser.ConstDecl:
			names := ""
			if len(s.Names) > 0 {
				names = s.Names[0]
			}
			t.errors = append(t.errors, fmt.Sprintf(
				"constants must be defined inside a class, found top-level constant '%s'", names))
		}
	}
}

// validateFileNaming 验证文件命名规则
// - 一个文件只能有一个 public class 或 public interface
// - public class/interface 名称必须与文件名一致
func (t *Transpiler) validateFileNaming(file *parser.File, fileName string) {
	var publicClasses []string
	var publicInterfaces []string

	for _, stmt := range file.Statements {
		switch s := stmt.(type) {
		case *parser.ClassDecl:
			if s.Public {
				publicClasses = append(publicClasses, s.Name)
			}
		case *parser.InterfaceDecl:
			if s.Public {
				publicInterfaces = append(publicInterfaces, s.Name)
			}
		}
	}

	// 检查数量
	totalPublic := len(publicClasses) + len(publicInterfaces)
	if totalPublic > 1 {
		t.errors = append(t.errors, fmt.Sprintf(
			"file '%s' has %d public classes/interfaces, only one is allowed",
			fileName, totalPublic))
		return
	}

	// 检查名称是否与文件名一致
	if len(publicClasses) == 1 && publicClasses[0] != fileName {
		t.errors = append(t.errors, fmt.Sprintf(
			"public class '%s' must be in file '%s.tugo', but found in '%s.tugo'",
			publicClasses[0], publicClasses[0], fileName))
	}
	if len(publicInterfaces) == 1 && publicInterfaces[0] != fileName {
		t.errors = append(t.errors, fmt.Sprintf(
			"public interface '%s' must be in file '%s.tugo', but found in '%s.tugo'",
			publicInterfaces[0], publicInterfaces[0], fileName))
	}
}

// validateMainMethod 验证入口类的 main 方法
// - main 方法必须是 public static
// - main 方法不能有参数和返回值
func (t *Transpiler) validateMainMethod(classDecl *parser.ClassDecl) {
	for _, method := range classDecl.Methods {
		if method.Name == "main" {
			// 检查是否是 static
			if !method.Static {
				t.errors = append(t.errors, fmt.Sprintf(
					"main method in class '%s' must be static", classDecl.Name))
			}
			// 检查是否是 public
			if method.Visibility != "public" {
				t.errors = append(t.errors, fmt.Sprintf(
					"main method in class '%s' must be public", classDecl.Name))
			}
			// 检查参数
			if len(method.Params) > 0 {
				t.errors = append(t.errors, fmt.Sprintf(
					"main method in class '%s' cannot have parameters", classDecl.Name))
			}
			// 检查返回值
			if len(method.Results) > 0 {
				t.errors = append(t.errors, fmt.Sprintf(
					"main method in class '%s' cannot have return values", classDecl.Name))
			}
			return
		}
	}
}

// IsEntryClass 检查是否是入口类（类名与文件名一致，且 package 是 main）
func (t *Transpiler) IsEntryClass(classDecl *parser.ClassDecl) bool {
	return t.pkg == "main" && t.currentFile != "" && classDecl.Name == t.currentFile
}
