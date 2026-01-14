package transpiler

import (
	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/i18n"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
)

// Transpiler 转译器
type Transpiler struct {
	table             *symbol.Table
	pkg               string
	imports           map[string]bool                    // 需要导入的包
	needFmt           bool                               // 是否需要导入 fmt
	funcDecls         map[string]*parser.FuncDecl        // 函数声明缓存 key: pkg.name
	classDecls        map[string]*parser.ClassDecl       // 类声明缓存 key: pkg.name
	interfaceDecls    map[string]*parser.InterfaceDecl   // 接口声明缓存 key: pkg.name
	structDecls       map[string]*parser.StructDecl      // 结构体声明缓存 key: pkg.name
	errors            []string                           // 转译错误
	config            *config.Config                     // 项目配置
	typeImports       map[string]string                  // 类型名到包名的映射 (User -> models)
	currentFile       string                             // 当前文件名（不含路径和后缀）
	skipValidation    bool                               // 跳过顶层语句验证（用于标准库）
	currentParsedFile *parser.File                       // 当前正在处理的文件
}

// AddError 添加转译错误
func (t *Transpiler) AddError(line, col int, msg string) {
	formatted := i18n.T(i18n.ErrGeneric, line, col, msg)
	t.errors = append(t.errors, formatted)
}

// New 创建一个新的转译器
func New(table *symbol.Table) *Transpiler {
	return &Transpiler{
		table:          table,
		imports:        make(map[string]bool),
		funcDecls:      make(map[string]*parser.FuncDecl),
		classDecls:     make(map[string]*parser.ClassDecl),
		interfaceDecls: make(map[string]*parser.InterfaceDecl),
		structDecls:    make(map[string]*parser.StructDecl),
		errors:         []string{},
		typeImports:    make(map[string]string),
	}
}

// SetConfig 设置项目配置
func (t *Transpiler) SetConfig(cfg *config.Config) {
	t.config = cfg
}

// SetSkipValidation 设置是否跳过顶层语句验证（用于标准库）
func (t *Transpiler) SetSkipValidation(skip bool) {
	t.skipValidation = skip
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
	t.currentParsedFile = file

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
		case *parser.StructDecl:
			key := t.pkg + "." + decl.Name
			t.structDecls[key] = decl
		}
	}

	// 验证顶层语句（禁止类外的 func/const/var）- 标准库跳过
	if !t.skipValidation {
		t.validateTopLevelStatements(file)

		// 验证文件命名规则（public class/interface 必须与文件名一致）
		if fileName != "" {
			t.validateFileNaming(file, fileName)
		}
	}

	// 校验 implements、extends、static class 和方法重载
	for _, stmt := range file.Statements {
		if classDecl, ok := stmt.(*parser.ClassDecl); ok {
			if classDecl.Static {
				t.validateStaticClass(classDecl)
			}
			if len(classDecl.Implements) > 0 {
				t.validateImplements(classDecl)
			}
			if classDecl.Extends != "" {
				t.validateExtends(classDecl, file)
			}
			// 验证入口类的 main 方法
			if t.IsEntryClass(classDecl) {
				t.validateMainMethod(classDecl)
			}
			// 验证方法重载的合法性
			t.validateOverloads(classDecl)
		}
		// 校验结构体
		if structDecl, ok := stmt.(*parser.StructDecl); ok {
			if len(structDecl.Implements) > 0 {
				t.validateStructImplements(structDecl)
			}
			// 验证结构体方法重载的合法性
			t.validateStructOverloads(structDecl)
		}
	}

	// 校验 errable 函数调用
	t.validateErrableCalls(file)
	
	// 校验方法/字段访问的可见性
	t.validateVisibility(file)
	
	// 校验未定义的符号（标准库跳过，因为同包内的类引用不需要导入）
	if !t.skipValidation {
		t.validateUndefinedSymbols(file)
	}
	
	// 校验未使用的导入
	t.validateUnusedImports(file)

	// 如果有错误，返回错误
	if len(t.errors) > 0 {
		return "", &ImplementsError{Errors: t.errors}
	}

	gen := NewCodeGen(t)
	code := gen.Generate(file)
	
	// 再次检查代码生成阶段的错误
	if len(t.errors) > 0 {
		return "", &ImplementsError{Errors: t.errors}
	}
	
	return code, nil
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

// GetStructDecl 获取结构体声明
func (t *Transpiler) GetStructDecl(pkg, name string) *parser.StructDecl {
	key := pkg + "." + name
	return t.structDecls[key]
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
			t.errors = append(t.errors, i18n.T(i18n.ErrInterfaceNotFound,
				classDecl.Name, ifaceName))
			continue
		}

		// 检查接口的每个方法
		for _, ifaceMethod := range ifaceInfo.Methods {
			classMethod, exists := classMethods[ifaceMethod.Name]
			if !exists {
				t.errors = append(t.errors, i18n.T(i18n.ErrMissingMethod,
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
		return i18n.T(i18n.ErrParamCountMismatch,
			className, method.Name, len(method.Params), ifaceName, len(sig.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(sig.Results) {
		return i18n.T(i18n.ErrReturnCountMismatch,
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
			t.errors = append(t.errors, i18n.T(i18n.ErrStructInterfaceNotFound,
				structDecl.Name, ifaceName))
			continue
		}

		// 检查接口的每个方法
		for _, ifaceMethod := range ifaceInfo.Methods {
			structMethod, exists := structMethods[ifaceMethod.Name]
			if !exists {
				t.errors = append(t.errors, i18n.T(i18n.ErrStructMissingMethod,
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
		return i18n.T(i18n.ErrStructParamMismatch,
			structName, method.Name, len(method.Params), ifaceName, len(sig.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(sig.Results) {
		return i18n.T(i18n.ErrStructReturnMismatch,
			structName, method.Name, len(method.Results), ifaceName, len(sig.Results))
	}

	return ""
}

// validateExtends 校验子类是否正确实现了父类的所有抽象方法
func (t *Transpiler) validateExtends(classDecl *parser.ClassDecl, file *parser.File) {
	// 查找父类信息
	parentInfo := t.table.GetClass(t.pkg, classDecl.Extends)
	if parentInfo == nil {
		// 检查是否是通过 use 导入的外部类（如 tugo.db.model）
		// 外部类无法验证，跳过检查
		if t.isImportedType(classDecl.Extends, file) {
			return
		}
		t.errors = append(t.errors, i18n.T(i18n.ErrParentClassNotFound,
			classDecl.Name, classDecl.Extends))
		return
	}

	// 只有抽象类需要检查方法实现
	if !parentInfo.Abstract {
		// 普通类继承：允许，无需额外检查
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
			t.errors = append(t.errors, i18n.T(i18n.ErrAbstractMethodMissing,
				classDecl.Name, abstractMethod.Name, classDecl.Extends))
			continue
		}

		// 校验方法签名
		if err := t.validateAbstractMethodSignature(classDecl.Name, classDecl.Extends, childMethod, abstractMethod); err != "" {
			t.errors = append(t.errors, err)
		}
	}
}

// isImportedType 检查类型是否是通过 use 导入的外部类型
func (t *Transpiler) isImportedType(typeName string, file *parser.File) bool {
	// 检查当前文件的 use 导入
	if file != nil {
		for _, importDecl := range file.Imports {
			for _, spec := range importDecl.Specs {
				// 只检查 tugo use 导入，不检查 Go import
				if spec.IsGoImport {
					continue
				}
				// use "tugo.db.model" 导入的 model
				if spec.TypeName == typeName {
					return true
				}
				// use "tugo.db.DB" as SomeAlias
				if spec.Alias != "" && spec.Alias == typeName {
					return true
				}
			}
		}
	}
	return false
}

// validateAbstractMethodSignature 校验抽象方法签名是否匹配
func (t *Transpiler) validateAbstractMethodSignature(className, parentName string, method *parser.ClassMethod, abstractMethod *parser.ClassMethod) string {
	// 检查参数数量
	if len(method.Params) != len(abstractMethod.Params) {
		return i18n.T(i18n.ErrAbstractParamMismatch,
			className, method.Name, len(method.Params), parentName, len(abstractMethod.Params))
	}

	// 检查返回值数量
	if len(method.Results) != len(abstractMethod.Results) {
		return i18n.T(i18n.ErrAbstractReturnMismatch,
			className, method.Name, len(method.Results), parentName, len(abstractMethod.Results))
	}

	return ""
}

// validateStaticClass 校验静态类
func (t *Transpiler) validateStaticClass(classDecl *parser.ClassDecl) {
	// 静态类不能有 init 构造函数
	if classDecl.InitMethod != nil {
		t.errors = append(t.errors, i18n.T(i18n.ErrStaticClassInit, classDecl.Name))
	}

	// 静态类方法体内不能使用 this
	for _, method := range classDecl.Methods {
		if method.Body != nil {
			if t.containsThis(method.Body) {
				t.errors = append(t.errors, i18n.T(i18n.ErrStaticClassThis,
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
			t.errors = append(t.errors, i18n.T(i18n.ErrTopLevelFunction, s.Name))
		case *parser.VarDecl:
			names := ""
			if len(s.Names) > 0 {
				names = s.Names[0]
			}
			t.errors = append(t.errors, i18n.T(i18n.ErrTopLevelVariable, names))
		case *parser.ConstDecl:
			names := ""
			if len(s.Names) > 0 {
				names = s.Names[0]
			}
			t.errors = append(t.errors, i18n.T(i18n.ErrTopLevelConstant, names))
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
		t.errors = append(t.errors, i18n.T(i18n.ErrTooManyPublicTypes,
			fileName, totalPublic))
		return
	}

	// 检查名称是否与文件名一致
	if len(publicClasses) == 1 && publicClasses[0] != fileName {
		t.errors = append(t.errors, i18n.T(i18n.ErrPublicClassFileName,
			publicClasses[0], publicClasses[0], fileName))
	}
	if len(publicInterfaces) == 1 && publicInterfaces[0] != fileName {
		t.errors = append(t.errors, i18n.T(i18n.ErrPublicIfaceFileName,
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
				t.errors = append(t.errors, i18n.T(i18n.ErrMainNotStatic, classDecl.Name))
			}
			// 检查是否是 public
			if method.Visibility != "public" {
				t.errors = append(t.errors, i18n.T(i18n.ErrMainNotPublic, classDecl.Name))
			}
			// 检查参数
			if len(method.Params) > 0 {
				t.errors = append(t.errors, i18n.T(i18n.ErrMainHasParams, classDecl.Name))
			}
			// 检查返回值
			if len(method.Results) > 0 {
				t.errors = append(t.errors, i18n.T(i18n.ErrMainHasReturns, classDecl.Name))
			}
			return
		}
	}
}

// IsEntryClass 检查是否是入口类（类名与文件名一致，且 package 是 main）
func (t *Transpiler) IsEntryClass(classDecl *parser.ClassDecl) bool {
	return t.pkg == "main" && t.currentFile != "" && classDecl.Name == t.currentFile
}

// validateErrableCalls 验证 errable 函数调用是否被正确处理
func (t *Transpiler) validateErrableCalls(file *parser.File) {
	for _, stmt := range file.Statements {
		switch s := stmt.(type) {
		case *parser.FuncDecl:
			if s.Body != nil {
				t.validateErrableCallsInFunc(s.Name, s.Errable, s.Body)
			}
		case *parser.ClassDecl:
			for _, method := range s.Methods {
				if method.Body != nil {
					t.validateErrableCallsInFunc(s.Name+"."+method.Name, method.Errable, method.Body)
				}
			}
			if s.InitMethod != nil && s.InitMethod.Body != nil {
				t.validateErrableCallsInFunc(s.Name+".init", s.InitMethod.Errable, s.InitMethod.Body)
			}
		case *parser.StructDecl:
			for _, method := range s.Methods {
				if method.Body != nil {
					t.validateErrableCallsInFunc(s.Name+"."+method.Name, method.Errable, method.Body)
				}
			}
			if s.InitMethod != nil && s.InitMethod.Body != nil {
				t.validateErrableCallsInFunc(s.Name+".init", s.InitMethod.Errable, s.InitMethod.Body)
			}
		}
	}
}

// validateErrableCallsInFunc 在函数/方法体中验证 errable 调用
func (t *Transpiler) validateErrableCallsInFunc(funcName string, funcIsErrable bool, body *parser.BlockStmt) {
	t.validateErrableCallsInBlock(funcName, funcIsErrable, false, body)
}

// validateErrableCallsInBlock 在代码块中验证 errable 调用
func (t *Transpiler) validateErrableCallsInBlock(funcName string, funcIsErrable bool, inTryBlock bool, block *parser.BlockStmt) {
	for _, stmt := range block.Statements {
		t.validateErrableCallsInStmt(funcName, funcIsErrable, inTryBlock, stmt)
	}
}

// validateErrableCallsInStmt 在语句中验证 errable 调用
func (t *Transpiler) validateErrableCallsInStmt(funcName string, funcIsErrable bool, inTryBlock bool, stmt parser.Statement) {
	switch s := stmt.(type) {
	case *parser.ExpressionStmt:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, s.Expression)
	case *parser.TryStmt:
		// try 块内的调用被允许
		if s.Body != nil {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, true, s.Body)
		}
		// catch 块内的调用也需要检查（catch 块不是 try 块）
		if s.Catch != nil && s.Catch.Body != nil {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, s.Catch.Body)
		}
	case *parser.ReturnStmt:
		for _, v := range s.Values {
			t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, v)
		}
	case *parser.AssignStmt:
		for _, r := range s.Right {
			t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, r)
		}
	case *parser.ShortVarDecl:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, s.Value)
	case *parser.VarDecl:
		if s.Value != nil {
			t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, s.Value)
		}
	case *parser.IfStmt:
		if s.Condition != nil {
			t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, s.Condition)
		}
		if s.Consequence != nil {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, s.Consequence)
		}
		if alt, ok := s.Alternative.(*parser.BlockStmt); ok {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, alt)
		} else if altIf, ok := s.Alternative.(*parser.IfStmt); ok {
			t.validateErrableCallsInStmt(funcName, funcIsErrable, inTryBlock, altIf)
		}
	case *parser.ForStmt:
		if s.Body != nil {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, s.Body)
		}
	case *parser.RangeStmt:
		if s.Body != nil {
			t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, s.Body)
		}
	case *parser.SwitchStmt:
		for _, c := range s.Cases {
			for _, stmt := range c.Body {
				t.validateErrableCallsInStmt(funcName, funcIsErrable, inTryBlock, stmt)
			}
		}
	case *parser.BlockStmt:
		t.validateErrableCallsInBlock(funcName, funcIsErrable, inTryBlock, s)
	}
}

// validateErrableCallsInExpr 在表达式中验证 errable 调用
func (t *Transpiler) validateErrableCallsInExpr(funcName string, funcIsErrable bool, inTryBlock bool, expr parser.Expression) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查被调用的函数是否是 errable
		if ident, ok := e.Function.(*parser.Identifier); ok {
			// 查找符号表
			sym := t.table.Get(t.pkg, ident.Value)
			if sym != nil && sym.Errable {
				// 这是一个 errable 调用，检查是否被正确处理
				if !inTryBlock && !funcIsErrable {
					t.errors = append(t.errors, i18n.T(i18n.ErrErrableNotHandled,
						funcName, ident.Value))
				}
			}
		} else if sel, ok := e.Function.(*parser.SelectorExpr); ok {
			// 方法调用 obj.method()
			methodName := sel.Sel
			// 尝试查找方法符号（需要知道接收者类型，这里简化处理）
			// 遍历所有符号查找匹配的方法
			for _, sym := range t.table.GetAll() {
				if sym.Kind == symbol.SymbolClassMethod || sym.Kind == symbol.SymbolMethod {
					if sym.Name == methodName && sym.Errable {
						if !inTryBlock && !funcIsErrable {
							t.errors = append(t.errors, i18n.T(i18n.ErrErrableMethodNotHandled,
								funcName, methodName))
						}
						break
					}
				}
			}
		}

		// 递归检查参数
		for _, arg := range e.Arguments {
			t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, arg)
		}
	case *parser.BinaryExpr:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.Left)
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.Right)
	case *parser.UnaryExpr:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.Operand)
	case *parser.IndexExpr:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.X)
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.Index)
	case *parser.SelectorExpr:
		t.validateErrableCallsInExpr(funcName, funcIsErrable, inTryBlock, e.X)
	}
}

// validateUndefinedSymbols 校验文件中使用的所有符号是否已定义或导入
func (t *Transpiler) validateUndefinedSymbols(file *parser.File) {
	// 收集所有导入的类型和包名
	importedTypes := make(map[string]bool)
	
	// 从导入语句收集
	for _, imp := range file.Imports {
		for _, spec := range imp.Specs {
			if spec.Alias != "" {
				// 使用别名
				importedTypes[spec.Alias] = true
			}
			// 导入的类型名
			if spec.TypeName != "" {
				importedTypes[spec.TypeName] = true
			}
			// Go import 语句：收集包名
			// 例如 import "com.example.orm/models" 应该注册 "models"
			if spec.IsGoImport && spec.PkgName != "" {
				importedTypes[spec.PkgName] = true
			}
		}
	}
	
	// 收集当前文件定义的类型
	definedTypes := make(map[string]bool)
	for _, stmt := range file.Statements {
		switch decl := stmt.(type) {
		case *parser.ClassDecl:
			definedTypes[decl.Name] = true
		case *parser.InterfaceDecl:
			definedTypes[decl.Name] = true
		case *parser.StructDecl:
			definedTypes[decl.Name] = true
		}
	}
	
	// 遍历所有语句，检查使用的类型
	for _, stmt := range file.Statements {
		t.validateSymbolsInStmt(stmt, importedTypes, definedTypes)
	}
}

// validateSymbolsInStmt 检查语句中使用的符号
func (t *Transpiler) validateSymbolsInStmt(stmt parser.Statement, importedTypes, definedTypes map[string]bool) {
	if stmt == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *parser.ClassDecl:
		// 检查类中的方法
		for _, method := range s.Methods {
			if method.Body != nil {
				t.validateSymbolsInBlock(method.Body, importedTypes, definedTypes)
			}
		}
	case *parser.FuncDecl:
		if s.Body != nil {
			t.validateSymbolsInBlock(s.Body, importedTypes, definedTypes)
		}
	case *parser.StructDecl:
		for _, method := range s.Methods {
			if method.Body != nil {
				t.validateSymbolsInBlock(method.Body, importedTypes, definedTypes)
			}
		}
	}
}

// validateSymbolsInBlock 检查块中使用的符号
func (t *Transpiler) validateSymbolsInBlock(block *parser.BlockStmt, importedTypes, definedTypes map[string]bool) {
	if block == nil {
		return
	}
	
	for _, stmt := range block.Statements {
		t.validateSymbolsInBlockStmt(stmt, importedTypes, definedTypes)
	}
}

// validateSymbolsInBlockStmt 检查块语句中使用的符号
func (t *Transpiler) validateSymbolsInBlockStmt(stmt parser.Statement, importedTypes, definedTypes map[string]bool) {
	if stmt == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *parser.ExpressionStmt:
		t.validateSymbolsInExpr(s.Expression, importedTypes, definedTypes)
	case *parser.ShortVarDecl:
		t.validateSymbolsInExpr(s.Value, importedTypes, definedTypes)
	case *parser.AssignStmt:
		for _, expr := range s.Left {
			t.validateSymbolsInExpr(expr, importedTypes, definedTypes)
		}
		for _, expr := range s.Right {
			t.validateSymbolsInExpr(expr, importedTypes, definedTypes)
		}
	case *parser.IfStmt:
		if s.Init != nil {
			t.validateSymbolsInBlockStmt(s.Init, importedTypes, definedTypes)
		}
		t.validateSymbolsInExpr(s.Condition, importedTypes, definedTypes)
		t.validateSymbolsInBlock(s.Consequence, importedTypes, definedTypes)
		if altBlock, ok := s.Alternative.(*parser.BlockStmt); ok {
			t.validateSymbolsInBlock(altBlock, importedTypes, definedTypes)
		} else if altIf, ok := s.Alternative.(*parser.IfStmt); ok {
			t.validateSymbolsInBlockStmt(altIf, importedTypes, definedTypes)
		}
	case *parser.ForStmt:
		if s.Init != nil {
			t.validateSymbolsInBlockStmt(s.Init, importedTypes, definedTypes)
		}
		t.validateSymbolsInExpr(s.Condition, importedTypes, definedTypes)
		if s.Post != nil {
			t.validateSymbolsInBlockStmt(s.Post, importedTypes, definedTypes)
		}
		t.validateSymbolsInBlock(s.Body, importedTypes, definedTypes)
	case *parser.ReturnStmt:
		for _, expr := range s.Values {
			t.validateSymbolsInExpr(expr, importedTypes, definedTypes)
		}
	case *parser.TryStmt:
		t.validateSymbolsInBlock(s.Body, importedTypes, definedTypes)
		if s.Catch != nil && s.Catch.Body != nil {
			t.validateSymbolsInBlock(s.Catch.Body, importedTypes, definedTypes)
		}
	case *parser.ThrowStmt:
		t.validateSymbolsInExpr(s.Value, importedTypes, definedTypes)
	}
}

// validateSymbolsInExpr 检查表达式中使用的符号
func (t *Transpiler) validateSymbolsInExpr(expr parser.Expression, importedTypes, definedTypes map[string]bool) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查构造函数调用 new Type()
		if newExpr, ok := e.Function.(*parser.NewExpr); ok {
			// new Type() 的情况
			if ident, ok := newExpr.Type.(*parser.Identifier); ok {
				typeName := ident.Value
				// 检查类型是否已定义或导入
				if !importedTypes[typeName] && !definedTypes[typeName] && !t.isBuiltinType(typeName) {
					t.errors = append(t.errors, i18n.T(i18n.ErrUndefinedType, typeName))
				}
			}
		} else {
			t.validateSymbolsInExpr(e.Function, importedTypes, definedTypes)
		}
		// 检查参数
		for _, arg := range e.Arguments {
			t.validateSymbolsInExpr(arg, importedTypes, definedTypes)
		}
	case *parser.NewExpr:
		// new 表达式
		if ident, ok := e.Type.(*parser.Identifier); ok {
			typeName := ident.Value
			if !importedTypes[typeName] && !definedTypes[typeName] && !t.isBuiltinType(typeName) {
				t.errors = append(t.errors, i18n.T(i18n.ErrUndefinedType, typeName))
			}
		}
	case *parser.BinaryExpr:
		t.validateSymbolsInExpr(e.Left, importedTypes, definedTypes)
		t.validateSymbolsInExpr(e.Right, importedTypes, definedTypes)
	case *parser.UnaryExpr:
		t.validateSymbolsInExpr(e.Operand, importedTypes, definedTypes)
	case *parser.IndexExpr:
		t.validateSymbolsInExpr(e.X, importedTypes, definedTypes)
		t.validateSymbolsInExpr(e.Index, importedTypes, definedTypes)
	case *parser.SelectorExpr:
		t.validateSymbolsInExpr(e.X, importedTypes, definedTypes)
	case *parser.StaticAccessExpr:
		// 静态访问 Str::ToUpper, 检查类名是否已导入或定义
		if ident, ok := e.Left.(*parser.Identifier); ok {
			typeName := ident.Value
			if !importedTypes[typeName] && !definedTypes[typeName] && !t.isBuiltinType(typeName) {
				t.errors = append(t.errors, i18n.T(i18n.ErrUndefinedType, typeName))
			}
		}
	}
}

// isBuiltinType 检查是否是内置类型
func (t *Transpiler) isBuiltinType(typeName string) bool {
	builtins := map[string]bool{
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true,
		"bool": true, "string": true, "byte": true, "rune": true,
		"error": true,
	}
	return builtins[typeName]
}

// validateUnusedImports 检查未使用的导入
func (t *Transpiler) validateUnusedImports(file *parser.File) {
	if len(file.Imports) == 0 {
		return
	}
	
	// 收集所有导入的类型
	importedTypes := make(map[string]*parser.ImportSpec)
	for _, imp := range file.Imports {
		for _, spec := range imp.Specs {
			// 跳过 Go 包导入（如 fmt，可能被 println 等内置函数使用）
			if spec.IsGoImport {
				continue
			}
			
			// 记录导入的类型
			key := spec.TypeName
			if spec.Alias != "" {
				key = spec.Alias
			}
			importedTypes[key] = spec
		}
	}
	
	if len(importedTypes) == 0 {
		return
	}
	
	// 检查代码中是否使用了这些类型
	usedTypes := make(map[string]bool)
	t.collectUsedTypes(file, usedTypes)
	
	// 检查未使用的导入
	for typeName, spec := range importedTypes {
		if !usedTypes[typeName] {
			t.errors = append(t.errors, i18n.T(i18n.ErrUnusedImport, typeName, spec.Path))
		}
	}
}

// collectUsedTypes 收集代码中使用的类型
func (t *Transpiler) collectUsedTypes(file *parser.File, usedTypes map[string]bool) {
	for _, stmt := range file.Statements {
		t.collectUsedTypesInStmt(stmt, usedTypes)
	}
}

// collectUsedTypesInStmt 在语句中收集使用的类型
func (t *Transpiler) collectUsedTypesInStmt(stmt parser.Statement, usedTypes map[string]bool) {
	if stmt == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *parser.ClassDecl:
		// 检查父类（extends）
		if s.Extends != "" {
			usedTypes[s.Extends] = true
		}
		// 检查实现的接口
		for _, impl := range s.Implements {
			usedTypes[impl] = true
		}
		// 检查类中的方法
		for _, method := range s.Methods {
			if method.Body != nil {
				t.collectUsedTypesInBlock(method.Body, usedTypes)
			}
		}
	case *parser.FuncDecl:
		if s.Body != nil {
			t.collectUsedTypesInBlock(s.Body, usedTypes)
		}
	case *parser.StructDecl:
		for _, method := range s.Methods {
			if method.Body != nil {
				t.collectUsedTypesInBlock(method.Body, usedTypes)
			}
		}
	}
}

// collectUsedTypesInBlock 在块中收集使用的类型
func (t *Transpiler) collectUsedTypesInBlock(block *parser.BlockStmt, usedTypes map[string]bool) {
	if block == nil {
		return
	}
	
	for _, stmt := range block.Statements {
		t.collectUsedTypesInBlockStmt(stmt, usedTypes)
	}
}

// collectUsedTypesInBlockStmt 在块语句中收集使用的类型
func (t *Transpiler) collectUsedTypesInBlockStmt(stmt parser.Statement, usedTypes map[string]bool) {
	if stmt == nil {
		return
	}
	
	switch s := stmt.(type) {
	case *parser.ExpressionStmt:
		t.collectUsedTypesInExpr(s.Expression, usedTypes)
	case *parser.ShortVarDecl:
		t.collectUsedTypesInExpr(s.Value, usedTypes)
	case *parser.VarDecl:
		if s.Value != nil {
			t.collectUsedTypesInExpr(s.Value, usedTypes)
		}
	case *parser.AssignStmt:
		for _, expr := range s.Left {
			t.collectUsedTypesInExpr(expr, usedTypes)
		}
		for _, expr := range s.Right {
			t.collectUsedTypesInExpr(expr, usedTypes)
		}
	case *parser.IfStmt:
		if s.Init != nil {
			t.collectUsedTypesInBlockStmt(s.Init, usedTypes)
		}
		t.collectUsedTypesInExpr(s.Condition, usedTypes)
		t.collectUsedTypesInBlock(s.Consequence, usedTypes)
		if altBlock, ok := s.Alternative.(*parser.BlockStmt); ok {
			t.collectUsedTypesInBlock(altBlock, usedTypes)
		} else if altIf, ok := s.Alternative.(*parser.IfStmt); ok {
			t.collectUsedTypesInBlockStmt(altIf, usedTypes)
		}
	case *parser.ForStmt:
		if s.Init != nil {
			t.collectUsedTypesInBlockStmt(s.Init, usedTypes)
		}
		t.collectUsedTypesInExpr(s.Condition, usedTypes)
		if s.Post != nil {
			t.collectUsedTypesInBlockStmt(s.Post, usedTypes)
		}
		t.collectUsedTypesInBlock(s.Body, usedTypes)
	case *parser.ReturnStmt:
		for _, expr := range s.Values {
			t.collectUsedTypesInExpr(expr, usedTypes)
		}
	case *parser.TryStmt:
		t.collectUsedTypesInBlock(s.Body, usedTypes)
		if s.Catch != nil && s.Catch.Body != nil {
			t.collectUsedTypesInBlock(s.Catch.Body, usedTypes)
		}
	case *parser.ThrowStmt:
		t.collectUsedTypesInExpr(s.Value, usedTypes)
	}
}

// collectUsedTypesInExpr 在表达式中收集使用的类型
func (t *Transpiler) collectUsedTypesInExpr(expr parser.Expression, usedTypes map[string]bool) {
	if expr == nil {
		return
	}
	
	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查构造函数调用 new Type()
		if newExpr, ok := e.Function.(*parser.NewExpr); ok {
			t.collectTypeNameFromExpr(newExpr.Type, usedTypes)
		} else if ident, ok := e.Function.(*parser.Identifier); ok {
			// 函数调用，如 getDB() - 标记函数名为使用
			usedTypes[ident.Value] = true
		} else {
			t.collectUsedTypesInExpr(e.Function, usedTypes)
		}
		// 检查参数
		for _, arg := range e.Arguments {
			t.collectUsedTypesInExpr(arg, usedTypes)
		}
	case *parser.NewExpr:
		// new 表达式
		t.collectTypeNameFromExpr(e.Type, usedTypes)
	case *parser.StaticAccessExpr:
		// 静态方法调用 Type::method
		if ident, ok := e.Left.(*parser.Identifier); ok {
			usedTypes[ident.Value] = true
		}
	case *parser.BinaryExpr:
		t.collectUsedTypesInExpr(e.Left, usedTypes)
		t.collectUsedTypesInExpr(e.Right, usedTypes)
	case *parser.UnaryExpr:
		t.collectUsedTypesInExpr(e.Operand, usedTypes)
	case *parser.IndexExpr:
		t.collectUsedTypesInExpr(e.X, usedTypes)
		t.collectUsedTypesInExpr(e.Index, usedTypes)
	case *parser.SelectorExpr:
		t.collectUsedTypesInExpr(e.X, usedTypes)
	case *parser.StructLiteral:
		// 结构体字面量 Type{...}
		t.collectTypeNameFromExpr(e.Type, usedTypes)
		for _, field := range e.Fields {
			t.collectUsedTypesInExpr(field.Value, usedTypes)
		}
	case *parser.Identifier:
		// 标识符可能是类型或变量，这里不处理（由其他地方处理）
	}
}

// collectTypeNameFromExpr 从类型表达式中提取类型名（支持泛型类型）
func (t *Transpiler) collectTypeNameFromExpr(typeExpr parser.Expression, usedTypes map[string]bool) {
	if typeExpr == nil {
		return
	}
	
	switch te := typeExpr.(type) {
	case *parser.Identifier:
		usedTypes[te.Value] = true
	case *parser.GenericType:
		// 泛型类型：从基础类型中提取类型名
		if ident, ok := te.Type.(*parser.Identifier); ok {
			usedTypes[ident.Value] = true
		}
	}
}

// validateOverloads 验证类方法重载的合法性
// 检查是否有重复的参数签名
func (t *Transpiler) validateOverloads(classDecl *parser.ClassDecl) {
	// 按方法名分组
	methodsByName := make(map[string][]*parser.ClassMethod)
	for _, method := range classDecl.Methods {
		methodsByName[method.Name] = append(methodsByName[method.Name], method)
	}

	// 检查每个方法名组
	for methodName, methods := range methodsByName {
		if len(methods) <= 1 {
			continue // 没有重载
		}

		// 检查签名唯一性
		signatures := make(map[string]bool)
		for _, method := range methods {
			sig := symbol.GenerateParamSignature(method.Params)
			if signatures[sig] {
				t.errors = append(t.errors, i18n.T(i18n.ErrDuplicateOverloadSignature,
					classDecl.Name, methodName, sig))
			}
			signatures[sig] = true
		}
	}
}

// validateStructOverloads 验证结构体方法重载的合法性
func (t *Transpiler) validateStructOverloads(structDecl *parser.StructDecl) {
	// 按方法名分组
	methodsByName := make(map[string][]*parser.ClassMethod)
	for _, method := range structDecl.Methods {
		methodsByName[method.Name] = append(methodsByName[method.Name], method)
	}

	// 检查每个方法名组
	for methodName, methods := range methodsByName {
		if len(methods) <= 1 {
			continue // 没有重载
		}

		// 检查签名唯一性
		signatures := make(map[string]bool)
		for _, method := range methods {
			sig := symbol.GenerateParamSignature(method.Params)
			if signatures[sig] {
				t.errors = append(t.errors, i18n.T(i18n.ErrDuplicateOverloadSignature,
					structDecl.Name, methodName, sig))
			}
			signatures[sig] = true
		}
	}
}

// validateVisibility 校验方法/字段访问的可见性
func (t *Transpiler) validateVisibility(file *parser.File) {
	// 从导入中构建类型到包名的映射
	typeToPackage := make(map[string]string)
	for _, impDecl := range file.Imports {
		for _, spec := range impDecl.Specs {
			if spec.IsGoImport {
				continue // 跳过 Go 包导入
			}
			// 获取类型名
			typeName := spec.TypeName
			if spec.Alias != "" {
				typeName = spec.Alias
			}
			typeToPackage[typeName] = spec.PkgName
		}
	}

	for _, stmt := range file.Statements {
		switch s := stmt.(type) {
		case *parser.ClassDecl:
			// 检查类中所有方法的方法调用
			for _, method := range s.Methods {
				if method.Body != nil {
					localVarTypes := make(map[string]string)
					t.validateVisibilityInBlock(s.Name, method.Body, localVarTypes, typeToPackage)
				}
			}
			if s.InitMethod != nil && s.InitMethod.Body != nil {
				localVarTypes := make(map[string]string)
				t.validateVisibilityInBlock(s.Name, s.InitMethod.Body, localVarTypes, typeToPackage)
			}
		case *parser.StructDecl:
			for _, method := range s.Methods {
				if method.Body != nil {
					localVarTypes := make(map[string]string)
					t.validateVisibilityInBlock(s.Name, method.Body, localVarTypes, typeToPackage)
				}
			}
			if s.InitMethod != nil && s.InitMethod.Body != nil {
				localVarTypes := make(map[string]string)
				t.validateVisibilityInBlock(s.Name, s.InitMethod.Body, localVarTypes, typeToPackage)
			}
		case *parser.FuncDecl:
			// 顶层函数（如 main）
			if s.Body != nil {
				localVarTypes := make(map[string]string)
				t.validateVisibilityInBlock("", s.Body, localVarTypes, typeToPackage)
			}
		}
	}
}

// validateVisibilityInBlock 在代码块中验证可见性
func (t *Transpiler) validateVisibilityInBlock(callerClass string, block *parser.BlockStmt, varTypes map[string]string, typeToPackage map[string]string) {
	for _, stmt := range block.Statements {
		t.validateVisibilityInStmt(callerClass, stmt, varTypes, typeToPackage)
	}
}

// validateVisibilityInStmt 在语句中验证可见性
func (t *Transpiler) validateVisibilityInStmt(callerClass string, stmt parser.Statement, varTypes map[string]string, typeToPackage map[string]string) {
	switch s := stmt.(type) {
	case *parser.ExpressionStmt:
		t.validateVisibilityInExpr(callerClass, s.Expression, varTypes, typeToPackage)
	case *parser.ReturnStmt:
		for _, v := range s.Values {
			t.validateVisibilityInExpr(callerClass, v, varTypes, typeToPackage)
		}
	case *parser.AssignStmt:
		for _, r := range s.Right {
			t.validateVisibilityInExpr(callerClass, r, varTypes, typeToPackage)
		}
	case *parser.ShortVarDecl:
		// 追踪变量类型
		if len(s.Names) == 1 {
			if typeName := t.inferExprType(s.Value); typeName != "" {
				varTypes[s.Names[0]] = typeName
			}
		}
		t.validateVisibilityInExpr(callerClass, s.Value, varTypes, typeToPackage)
	case *parser.VarDecl:
		if s.Value != nil {
			// 追踪变量类型
			if len(s.Names) == 1 {
				if typeName := t.inferExprType(s.Value); typeName != "" {
					varTypes[s.Names[0]] = typeName
				}
			}
			t.validateVisibilityInExpr(callerClass, s.Value, varTypes, typeToPackage)
		}
	case *parser.IfStmt:
		if s.Condition != nil {
			t.validateVisibilityInExpr(callerClass, s.Condition, varTypes, typeToPackage)
		}
		if s.Consequence != nil {
			t.validateVisibilityInBlock(callerClass, s.Consequence, varTypes, typeToPackage)
		}
		if alt, ok := s.Alternative.(*parser.BlockStmt); ok {
			t.validateVisibilityInBlock(callerClass, alt, varTypes, typeToPackage)
		} else if altIf, ok := s.Alternative.(*parser.IfStmt); ok {
			t.validateVisibilityInStmt(callerClass, altIf, varTypes, typeToPackage)
		}
	case *parser.ForStmt:
		if s.Body != nil {
			t.validateVisibilityInBlock(callerClass, s.Body, varTypes, typeToPackage)
		}
	case *parser.RangeStmt:
		if s.Body != nil {
			t.validateVisibilityInBlock(callerClass, s.Body, varTypes, typeToPackage)
		}
	case *parser.TryStmt:
		if s.Body != nil {
			t.validateVisibilityInBlock(callerClass, s.Body, varTypes, typeToPackage)
		}
		if s.Catch != nil && s.Catch.Body != nil {
			t.validateVisibilityInBlock(callerClass, s.Catch.Body, varTypes, typeToPackage)
		}
	case *parser.BlockStmt:
		t.validateVisibilityInBlock(callerClass, s, varTypes, typeToPackage)
	}
}

// validateVisibilityInExpr 在表达式中验证可见性
func (t *Transpiler) validateVisibilityInExpr(callerClass string, expr parser.Expression, varTypes map[string]string, typeToPackage map[string]string) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *parser.CallExpr:
		// 检查方法调用
		if sel, ok := e.Function.(*parser.SelectorExpr); ok {
			t.checkMethodCallVisibility(callerClass, sel, e, varTypes, typeToPackage)
		}
		// 检查参数
		for _, arg := range e.Arguments {
			t.validateVisibilityInExpr(callerClass, arg, varTypes, typeToPackage)
		}
	case *parser.SelectorExpr:
		// 检查字段访问（不是方法调用的情况）
		t.validateVisibilityInExpr(callerClass, e.X, varTypes, typeToPackage)
	case *parser.BinaryExpr:
		t.validateVisibilityInExpr(callerClass, e.Left, varTypes, typeToPackage)
		t.validateVisibilityInExpr(callerClass, e.Right, varTypes, typeToPackage)
	case *parser.UnaryExpr:
		t.validateVisibilityInExpr(callerClass, e.Operand, varTypes, typeToPackage)
	case *parser.IndexExpr:
		t.validateVisibilityInExpr(callerClass, e.X, varTypes, typeToPackage)
		t.validateVisibilityInExpr(callerClass, e.Index, varTypes, typeToPackage)
	case *parser.NewExpr:
		for _, arg := range e.Arguments {
			t.validateVisibilityInExpr(callerClass, arg, varTypes, typeToPackage)
		}
	}
}

// checkMethodCallVisibility 检查方法调用的可见性
func (t *Transpiler) checkMethodCallVisibility(callerClass string, sel *parser.SelectorExpr, call *parser.CallExpr, varTypes map[string]string, typeToPackage map[string]string) {
	methodName := sel.Sel

	// 获取接收者的类型
	receiverType := t.inferReceiverTypeWithVars(sel.X, varTypes, typeToPackage)
	if receiverType == "" {
		return // 无法确定类型，跳过检查
	}

	// 如果调用者和被调用者是同一个类，允许访问私有方法
	if receiverType == callerClass {
		return
	}

	// 查找目标类的方法，检查可见性
	// 首先在当前包中查找
	classInfo := t.table.GetClass(t.pkg, receiverType)
	if classInfo == nil {
		// 尝试从类型导入映射中获取包名
		if pkgName, ok := typeToPackage[receiverType]; ok {
			classInfo = t.table.GetClass(pkgName, receiverType)
		}
	}

	if classInfo != nil {
		// 检查方法
		for _, method := range classInfo.Methods {
			if method.Name == methodName {
				isPublic := method.Visibility == "public" || method.Visibility == "protected"
				if !isPublic {
					callerName := callerClass
					if callerName == "" {
						callerName = "main"
					}
					t.errors = append(t.errors, i18n.T(i18n.ErrPrivateMethodAccess,
						callerName, receiverType, methodName))
				}
				return
			}
		}
	}
}

// inferExprType 推断表达式的类型（用于变量追踪）
func (t *Transpiler) inferExprType(expr parser.Expression) string {
	switch e := expr.(type) {
	case *parser.NewExpr:
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return ident.Value
		}
	case *parser.CallExpr:
		// 函数调用返回值，检查是否是 NewXxx 模式
		if ident, ok := e.Function.(*parser.Identifier); ok {
			funcName := ident.Value
			if len(funcName) > 3 && funcName[:3] == "New" {
				return funcName[3:]
			}
		}
	case *parser.StructLiteral:
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return ident.Value
		}
	}
	return ""
}

// inferReceiverTypeWithVars 推断表达式的类型（使用变量类型表）
func (t *Transpiler) inferReceiverTypeWithVars(expr parser.Expression, varTypes map[string]string, typeToPackage map[string]string) string {
	switch e := expr.(type) {
	case *parser.Identifier:
		// 首先检查变量类型表
		if typeName, ok := varTypes[e.Value]; ok {
			return typeName
		}
		// 检查是否是类名
		if classInfo := t.table.GetClass(t.pkg, e.Value); classInfo != nil {
			return e.Value
		}
		return ""
	case *parser.NewExpr:
		if ident, ok := e.Type.(*parser.Identifier); ok {
			return ident.Value
		}
	case *parser.ThisExpr:
		// this 表达式，返回当前上下文的类名（需要传递上下文）
		return ""
	case *parser.CallExpr:
		// 函数调用返回值，检查是否是 NewXxx 模式
		if ident, ok := e.Function.(*parser.Identifier); ok {
			funcName := ident.Value
			if len(funcName) > 3 && funcName[:3] == "New" {
				typeName := funcName[3:]
				// 检查在当前包或导入的包中是否存在该类
				if t.table.GetClass(t.pkg, typeName) != nil {
					return typeName
				}
				for pkgName := range typeToPackage {
					if t.table.GetClass(pkgName, typeName) != nil {
						return typeName
					}
				}
			}
		}
	}
	return ""
}

// GetExternalClass 获取外部包的类信息
// 用于在生成继承方法包装时获取外部父类的方法信息
func (t *Transpiler) GetExternalClass(pkgName, className string) *symbol.ClassInfo {
	// 首先尝试从符号表获取
	classInfo := t.table.GetClass(pkgName, className)
	if classInfo != nil {
		return classInfo
	}

	// 尝试从类声明缓存获取
	key := pkgName + "." + className
	if classDecl, ok := t.classDecls[key]; ok {
		// 转换为 ClassInfo
		return &symbol.ClassInfo{
			Name:            classDecl.Name,
			GoName:          symbol.ToGoName(classDecl.Name, classDecl.Public),
			Public:          classDecl.Public,
			Abstract:        classDecl.Abstract,
			Package:         pkgName,
			Extends:         classDecl.Extends,
			Implements:      classDecl.Implements,
			Fields:          classDecl.Fields,
			Methods:         classDecl.Methods,
			AbstractMethods: classDecl.AbstractMethods,
			InitMethod:      classDecl.InitMethod,
		}
	}

	return nil
}
