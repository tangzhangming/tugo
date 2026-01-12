package transpiler

import (
	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/i18n"
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
	skipValidation bool                               // 跳过顶层语句验证（用于标准库）
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

	// 验证顶层语句（禁止类外的 func/const/var）- 标准库跳过
	if !t.skipValidation {
		t.validateTopLevelStatements(file)

		// 验证文件命名规则（public class/interface 必须与文件名一致）
		if fileName != "" {
			t.validateFileNaming(file, fileName)
		}
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

	// 校验 errable 函数调用
	t.validateErrableCalls(file)
	
	// 校验未定义的符号
	t.validateUndefinedSymbols(file)
	
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
func (t *Transpiler) validateExtends(classDecl *parser.ClassDecl) {
	// 查找父类信息
	parentInfo := t.table.GetClass(t.pkg, classDecl.Extends)
	if parentInfo == nil {
		t.errors = append(t.errors, i18n.T(i18n.ErrParentClassNotFound,
			classDecl.Name, classDecl.Extends))
		return
	}

	// 父类必须是抽象类
	if !parentInfo.Abstract {
		t.errors = append(t.errors, i18n.T(i18n.ErrExtendNonAbstract,
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
	// 收集所有导入的类型
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
			// 跳过 Go 标准库导入（如 fmt，可能被 println 等内置函数使用）
			if spec.FromGo {
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
			if ident, ok := newExpr.Type.(*parser.Identifier); ok {
				usedTypes[ident.Value] = true
			}
		} else {
			t.collectUsedTypesInExpr(e.Function, usedTypes)
		}
		// 检查参数
		for _, arg := range e.Arguments {
			t.collectUsedTypesInExpr(arg, usedTypes)
		}
	case *parser.NewExpr:
		// new 表达式
		if ident, ok := e.Type.(*parser.Identifier); ok {
			usedTypes[ident.Value] = true
		}
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
	}
}
