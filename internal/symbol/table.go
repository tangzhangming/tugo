package symbol

import (
	"strings"
	"unicode"

	"github.com/tangzhangming/tugo/internal/parser"
)

// SymbolKind 符号类型
type SymbolKind int

const (
	SymbolFunc SymbolKind = iota
	SymbolStruct
	SymbolInterface
	SymbolType
	SymbolVar
	SymbolConst
	SymbolMethod
	SymbolClass
	SymbolClassMethod
)

// Symbol 表示一个符号
type Symbol struct {
	Name        string     // 原始名称
	GoName      string     // 翻译后的 Go 名称
	Kind        SymbolKind // 符号类型
	Public      bool       // 是否公开
	Package     string     // 所属包
	Receiver    string     // 方法的接收者类型（仅用于方法）
	HasDefault  bool       // 函数是否有默认参数
	Errable     bool       // 是否可能抛出错误（返回类型带 ! 标记）
	ResultCount int        // 返回值数量（不包括error）
}

// InterfaceInfo 存储接口的完整信息
type InterfaceInfo struct {
	Name    string
	GoName  string
	Public  bool
	Package string
	Methods []*parser.FuncSignature
}

// ClassInfo 存储类的完整信息
type ClassInfo struct {
	Name            string
	GoName          string
	Public          bool
	Abstract        bool
	Package         string
	Extends         string              // 父类名
	Implements      []string            // 实现的接口
	Fields          []*parser.ClassField
	Methods         []*parser.ClassMethod
	AbstractMethods []*parser.ClassMethod
	InitMethod      *parser.ClassMethod
}

// Table 符号表
type Table struct {
	symbols    map[string]*Symbol         // key: package.name 或 package.receiver.name（方法）
	packages   map[string]bool            // 所有包名
	interfaces map[string]*InterfaceInfo  // key: package.name
	classes    map[string]*ClassInfo      // key: package.name
}

// New 创建一个新的符号表
func New() *Table {
	return &Table{
		symbols:    make(map[string]*Symbol),
		packages:   make(map[string]bool),
		interfaces: make(map[string]*InterfaceInfo),
		classes:    make(map[string]*ClassInfo),
	}
}

// key 生成符号的键
func key(pkg, name string) string {
	return pkg + "." + name
}

// methodKey 生成方法的键
func methodKey(pkg, receiver, name string) string {
	return pkg + "." + receiver + "." + name
}

// Add 添加一个符号
func (t *Table) Add(sym *Symbol) {
	k := key(sym.Package, sym.Name)
	if sym.Kind == SymbolMethod && sym.Receiver != "" {
		k = methodKey(sym.Package, sym.Receiver, sym.Name)
	}
	t.symbols[k] = sym
	t.packages[sym.Package] = true
}

// Get 获取一个符号
func (t *Table) Get(pkg, name string) *Symbol {
	return t.symbols[key(pkg, name)]
}

// GetMethod 获取一个方法
func (t *Table) GetMethod(pkg, receiver, name string) *Symbol {
	return t.symbols[methodKey(pkg, receiver, name)]
}

// GetMethodByName 根据方法名查找方法（遍历所有接收者）
func (t *Table) GetMethodByName(pkg, name string) *Symbol {
	// 遍历所有符号，查找匹配的方法
	for k, sym := range t.symbols {
		if sym.Kind == SymbolMethod || sym.Kind == SymbolClassMethod {
			if sym.Name == name {
				// 检查包名是否匹配（可以是当前包或任何包）
				if sym.Package == pkg || strings.HasPrefix(k, pkg+".") {
					return sym
				}
			}
		}
	}
	// 如果在当前包没找到，尝试在所有包中查找
	for _, sym := range t.symbols {
		if (sym.Kind == SymbolMethod || sym.Kind == SymbolClassMethod) && sym.Name == name {
			return sym
		}
	}
	return nil
}

// AddInterface 添加接口信息
func (t *Table) AddInterface(info *InterfaceInfo) {
	k := key(info.Package, info.Name)
	t.interfaces[k] = info
}

// GetInterface 获取接口信息
func (t *Table) GetInterface(pkg, name string) *InterfaceInfo {
	return t.interfaces[key(pkg, name)]
}

// AddClass 添加类信息
func (t *Table) AddClass(info *ClassInfo) {
	k := key(info.Package, info.Name)
	t.classes[k] = info
}

// GetClass 获取类信息
func (t *Table) GetClass(pkg, name string) *ClassInfo {
	return t.classes[key(pkg, name)]
}

// GetAll 获取所有符号
func (t *Table) GetAll() []*Symbol {
	result := make([]*Symbol, 0, len(t.symbols))
	for _, sym := range t.symbols {
		result = append(result, sym)
	}
	return result
}

// GetByPackage 获取指定包的所有符号
func (t *Table) GetByPackage(pkg string) []*Symbol {
	var result []*Symbol
	for _, sym := range t.symbols {
		if sym.Package == pkg {
			result = append(result, sym)
		}
	}
	return result
}

// ToGoName 将 tugo 名称转换为 Go 名称
func ToGoName(name string, public bool) string {
	// 处理 $ 开头的变量名
	if strings.HasPrefix(name, "$") {
		name = "_tugo_" + name[1:]
	}

	if public {
		// 首字母大写
		return capitalize(name)
	}
	// 确保首字母小写
	return uncapitalize(name)
}

// capitalize 首字母大写
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// uncapitalize 首字母小写
func uncapitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// TransformDollarVar 转换 $ 变量名
func TransformDollarVar(name string) string {
	if strings.HasPrefix(name, "$") {
		return "_tugo_" + name[1:]
	}
	return name
}

// Collector 符号收集器
type Collector struct {
	table   *Table
	pkg     string
	imports map[string]string // 导入的包别名 -> 路径
}

// NewCollector 创建一个新的符号收集器
func NewCollector(table *Table) *Collector {
	return &Collector{
		table:   table,
		imports: make(map[string]string),
	}
}

// CollectFile 从文件 AST 收集符号
func (c *Collector) CollectFile(file *parser.File) {
	c.pkg = file.Package

	// 收集导入
	for _, imp := range file.Imports {
		for _, spec := range imp.Specs {
			path := strings.Trim(spec.Path, "\"")
			alias := spec.Alias
			if alias == "" {
				// 使用路径的最后一部分作为别名
				parts := strings.Split(path, "/")
				alias = parts[len(parts)-1]
			}
			c.imports[alias] = path
		}
	}

	// 收集语句中的符号
	for _, stmt := range file.Statements {
		c.collectStatement(stmt)
	}
}

// collectStatement 从语句中收集符号
func (c *Collector) collectStatement(stmt parser.Statement) {
	switch s := stmt.(type) {
	case *parser.FuncDecl:
		c.collectFunc(s)
	case *parser.StructDecl:
		c.collectStruct(s)
	case *parser.ClassDecl:
		c.collectClass(s)
	case *parser.InterfaceDecl:
		c.collectInterface(s)
	case *parser.TypeDecl:
		c.collectType(s)
	case *parser.VarDecl:
		c.collectVar(s)
	case *parser.ConstDecl:
		c.collectConst(s)
	}
}

// collectFunc 收集函数符号
func (c *Collector) collectFunc(decl *parser.FuncDecl) {
	sym := &Symbol{
		Name:        decl.Name,
		GoName:      ToGoName(decl.Name, decl.Public),
		Kind:        SymbolFunc,
		Public:      decl.Public,
		Package:     c.pkg,
		Errable:     decl.Errable,
		ResultCount: len(decl.Results),
	}

	// 检查是否有默认参数
	for _, param := range decl.Params {
		if param.DefaultValue != nil {
			sym.HasDefault = true
			break
		}
	}

	// 检查是否是方法
	if decl.Receiver != nil {
		sym.Kind = SymbolMethod
		if ident, ok := decl.Receiver.Type.(*parser.Identifier); ok {
			sym.Receiver = ident.Value
		} else if ptr, ok := decl.Receiver.Type.(*parser.PointerType); ok {
			if ident, ok := ptr.Base.(*parser.Identifier); ok {
				sym.Receiver = ident.Value
			}
		}
	}

	c.table.Add(sym)
}

// collectStruct 收集结构体符号
func (c *Collector) collectStruct(decl *parser.StructDecl) {
	sym := &Symbol{
		Name:    decl.Name,
		GoName:  ToGoName(decl.Name, decl.Public),
		Kind:    SymbolStruct,
		Public:  decl.Public,
		Package: c.pkg,
	}
	c.table.Add(sym)
}

// collectClass 收集类符号
func (c *Collector) collectClass(decl *parser.ClassDecl) {
	// 收集类本身
	sym := &Symbol{
		Name:    decl.Name,
		GoName:  ToGoName(decl.Name, decl.Public),
		Kind:    SymbolClass,
		Public:  decl.Public,
		Package: c.pkg,
	}
	c.table.Add(sym)

	// 存储类的完整信息
	classInfo := &ClassInfo{
		Name:            decl.Name,
		GoName:          ToGoName(decl.Name, decl.Public),
		Public:          decl.Public,
		Abstract:        decl.Abstract,
		Package:         c.pkg,
		Extends:         decl.Extends,
		Implements:      decl.Implements,
		Fields:          decl.Fields,
		Methods:         decl.Methods,
		AbstractMethods: decl.AbstractMethods,
		InitMethod:      decl.InitMethod,
	}
	c.table.AddClass(classInfo)

	// 收集类方法
	for _, method := range decl.Methods {
		isPublic := method.Visibility == "public" || method.Visibility == "protected"
		methodSym := &Symbol{
			Name:        method.Name,
			GoName:      ToGoName(method.Name, isPublic),
			Kind:        SymbolClassMethod,
			Public:      isPublic,
			Package:     c.pkg,
			Receiver:    decl.Name,
			Errable:     method.Errable,
			ResultCount: len(method.Results),
		}

		// 检查是否有默认参数
		for _, param := range method.Params {
			if param.DefaultValue != nil {
				methodSym.HasDefault = true
				break
			}
		}

		c.table.Add(methodSym)
	}

	// 收集抽象方法
	for _, method := range decl.AbstractMethods {
		isPublic := method.Visibility == "public" || method.Visibility == "protected"
		methodSym := &Symbol{
			Name:        method.Name,
			GoName:      ToGoName(method.Name, isPublic),
			Kind:        SymbolClassMethod,
			Public:      isPublic,
			Package:     c.pkg,
			Receiver:    decl.Name,
			Errable:     method.Errable,
			ResultCount: len(method.Results),
		}
		c.table.Add(methodSym)
	}

	// 收集构造方法
	if decl.InitMethod != nil {
		initSym := &Symbol{
			Name:     "init",
			GoName:   "New" + ToGoName(decl.Name, decl.Public),
			Kind:     SymbolClassMethod,
			Public:   true,
			Package:  c.pkg,
			Receiver: decl.Name,
			Errable:  decl.InitMethod.Errable,
		}
		for _, param := range decl.InitMethod.Params {
			if param.DefaultValue != nil {
				initSym.HasDefault = true
				break
			}
		}
		c.table.Add(initSym)
	}
}

// collectInterface 收集接口符号
func (c *Collector) collectInterface(decl *parser.InterfaceDecl) {
	sym := &Symbol{
		Name:    decl.Name,
		GoName:  ToGoName(decl.Name, decl.Public),
		Kind:    SymbolInterface,
		Public:  decl.Public,
		Package: c.pkg,
	}
	c.table.Add(sym)

	// 存储接口方法签名信息
	info := &InterfaceInfo{
		Name:    decl.Name,
		GoName:  ToGoName(decl.Name, decl.Public),
		Public:  decl.Public,
		Package: c.pkg,
		Methods: decl.Methods,
	}
	c.table.AddInterface(info)
}

// collectType 收集类型符号
func (c *Collector) collectType(decl *parser.TypeDecl) {
	sym := &Symbol{
		Name:    decl.Name,
		GoName:  ToGoName(decl.Name, decl.Public),
		Kind:    SymbolType,
		Public:  decl.Public,
		Package: c.pkg,
	}
	c.table.Add(sym)
}

// collectVar 收集变量符号
func (c *Collector) collectVar(decl *parser.VarDecl) {
	for _, name := range decl.Names {
		sym := &Symbol{
			Name:    name,
			GoName:  TransformDollarVar(name),
			Kind:    SymbolVar,
			Public:  false, // 包级变量的可见性需要另外处理
			Package: c.pkg,
		}
		c.table.Add(sym)
	}
}

// collectConst 收集常量符号
func (c *Collector) collectConst(decl *parser.ConstDecl) {
	for _, name := range decl.Names {
		sym := &Symbol{
			Name:    name,
			GoName:  TransformDollarVar(name),
			Kind:    SymbolConst,
			Public:  false,
			Package: c.pkg,
		}
		c.table.Add(sym)
	}
}

// Collect 从多个文件收集符号
func Collect(files []*parser.File) *Table {
	table := New()
	collector := NewCollector(table)
	for _, file := range files {
		collector.CollectFile(file)
	}
	return table
}
