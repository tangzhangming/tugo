package parser

import (
	"github.com/tangzhangming/tugo/internal/lexer"
)

// Node AST 节点接口
type Node interface {
	TokenLiteral() string
}

// Statement 语句接口
type Statement interface {
	Node
	statementNode()
}

// Expression 表达式接口
type Expression interface {
	Node
	expressionNode()
}

// File 表示一个源文件
type File struct {
	Package    string
	Imports    []*ImportDecl
	Statements []Statement
}

func (f *File) TokenLiteral() string { return "file" }

// ImportDecl 导入声明
type ImportDecl struct {
	Token lexer.Token // import token
	Specs []*ImportSpec
}

func (i *ImportDecl) TokenLiteral() string { return i.Token.Literal }
func (i *ImportDecl) statementNode()       {}

// ImportSpec 单个导入项
type ImportSpec struct {
	Alias      string // 别名（可选）
	Path       string // 完整导入路径 (如 "com.company.demo.models.User" 或 "fmt")
	TypeName   string // 类型名（use语句：最后一截，如 "User"）
	PkgPath    string // 包路径（use语句：去掉类型名后，如 "com.company.demo.models"）
	PkgName    string // 包名（use语句：最后一个目录名，如 "models"）
	IsGoImport bool   // true=Go包(import语句), false=tugo包(use语句)
}

// FuncDecl 函数声明
type FuncDecl struct {
	Token      lexer.Token    // func token
	Public     bool           // 是否公开
	Name       string         // 函数名
	Receiver   *Field         // 接收者（方法时使用）
	TypeParams *TypeParamList // 泛型类型参数（可选）
	Params     []*Field       // 参数列表
	Results    []*Field       // 返回值列表
	Body       *BlockStmt     // 函数体
	Errable    bool           // 是否可能抛出错误（返回类型带 ! 标记）
}

func (f *FuncDecl) TokenLiteral() string { return f.Token.Literal }
func (f *FuncDecl) statementNode()       {}

// Field 表示参数或返回值
type Field struct {
	Name         string     // 名称
	Type         Expression // 类型
	DefaultValue Expression // 默认值（可选）
}

// StructDecl 结构体声明
type StructDecl struct {
	Token      lexer.Token    // struct token
	Public     bool           // 是否公开
	Name       string         // 结构体名
	TypeParams *TypeParamList // 泛型类型参数（可选）
	Implements []string       // 实现的接口列表
	Fields     []*StructField // 字段列表
	Embeds     []string       // 嵌入的类型列表（匿名字段）
	Methods    []*ClassMethod // 方法列表（复用 ClassMethod）
	InitMethod *ClassMethod   // 构造方法
}

func (s *StructDecl) TokenLiteral() string { return s.Token.Literal }
func (s *StructDecl) statementNode()       {}

// FieldTag 字段标签
type FieldTag struct {
	Key   string // 标签键 (如 "json", "validate")
	Value string // 标签值 (如 "user_name", "min=18,max=100")
}

// StructField 结构体字段
type StructField struct {
	Name       string      // 字段名
	Type       Expression  // 类型
	Tag        string      // 标签（旧版，保留兼容）
	Tags       []*FieldTag // 标签列表（新版）
	Public     bool        // 是否公开
	Visibility string      // public/private（可选，默认 private）
}

// ClassDecl 类声明
type ClassDecl struct {
	Token           lexer.Token    // class token
	Public          bool           // 是否公开
	Abstract        bool           // 是否抽象类
	Static          bool           // 是否静态类
	Name            string         // 类名
	TypeParams      *TypeParamList // 泛型类型参数（可选）
	Extends         string         // 继承的父类（可选）
	Implements      []string       // 实现的接口列表
	Fields          []*ClassField  // 字段列表
	Methods         []*ClassMethod // 方法列表
	AbstractMethods []*ClassMethod // 抽象方法列表
	InitMethod      *ClassMethod   // 构造方法（兼容旧代码，取 InitMethods[0]）
	InitMethods     []*ClassMethod // 多个构造方法（重载）
}

func (c *ClassDecl) TokenLiteral() string { return c.Token.Literal }
func (c *ClassDecl) statementNode()       {}

// ClassField 类字段
type ClassField struct {
	Name       string      // 字段名
	Type       Expression  // 类型
	Value      Expression  // 默认值（可选）
	Visibility string      // public/private/protected
	Static     bool        // 是否静态
	Tags       []*FieldTag // 标签列表
}

// ClassMethod 类方法
type ClassMethod struct {
	Token      lexer.Token    // func token
	Name       string         // 方法名
	TypeParams *TypeParamList // 泛型类型参数（可选）
	Params     []*Field       // 参数列表
	Results    []*Field       // 返回值列表
	Body       *BlockStmt     // 方法体（抽象方法为 nil）
	Visibility string         // public/private/protected
	Static     bool           // 是否静态
	Abstract   bool           // 是否抽象方法
	Errable    bool           // 是否可能抛出错误（返回类型带 ! 标记）
}

// ThisExpr this 表达式
type ThisExpr struct {
	Token lexer.Token
}

func (t *ThisExpr) TokenLiteral() string { return t.Token.Literal }
func (t *ThisExpr) expressionNode()      {}

// SelfExpr self 表达式（用于静态类）
type SelfExpr struct {
	Token lexer.Token
}

func (s *SelfExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SelfExpr) expressionNode()      {}

// StaticAccessExpr 静态访问表达式 (ClassName::member 或 self::member)
type StaticAccessExpr struct {
	Token  lexer.Token // :: token
	Left   Expression  // 类名或 self
	Member string      // 成员名
}

func (s *StaticAccessExpr) TokenLiteral() string { return s.Token.Literal }
func (s *StaticAccessExpr) expressionNode()      {}

// InterfaceDecl 接口声明
type InterfaceDecl struct {
	Token      lexer.Token
	Public     bool
	Name       string
	TypeParams *TypeParamList // 泛型类型参数（可选）
	Methods    []*FuncSignature
}

func (i *InterfaceDecl) TokenLiteral() string { return i.Token.Literal }
func (i *InterfaceDecl) statementNode()       {}

// FuncSignature 函数签名（用于接口）
type FuncSignature struct {
	Name    string
	Params  []*Field
	Results []*Field
	Errable bool // 是否可能抛出错误（返回类型带 ! 标记）
}

// TypeDecl 类型声明
type TypeDecl struct {
	Token      lexer.Token
	Public     bool
	Name       string
	TypeParams *TypeParamList // 泛型类型参数（可选）
	Type       Expression
}

func (t *TypeDecl) TokenLiteral() string { return t.Token.Literal }
func (t *TypeDecl) statementNode()       {}

// VarDecl 变量声明
type VarDecl struct {
	Token lexer.Token // var token
	Names []string    // 变量名列表
	Type  Expression  // 类型（可选）
	Value Expression  // 初始值（可选）
}

func (v *VarDecl) TokenLiteral() string { return v.Token.Literal }
func (v *VarDecl) statementNode()       {}

// ConstDecl 常量声明
type ConstDecl struct {
	Token lexer.Token
	Names []string
	Type  Expression
	Value Expression
}

func (c *ConstDecl) TokenLiteral() string { return c.Token.Literal }
func (c *ConstDecl) statementNode()       {}

// ShortVarDecl 短变量声明 :=
type ShortVarDecl struct {
	Token lexer.Token // := token
	Names []string    // 变量名列表
	Value Expression  // 值
}

func (s *ShortVarDecl) TokenLiteral() string { return s.Token.Literal }
func (s *ShortVarDecl) statementNode()       {}

// AssignStmt 赋值语句
type AssignStmt struct {
	Token lexer.Token  // = token
	Left  []Expression // 左侧表达式
	Right []Expression // 右侧表达式
}

func (a *AssignStmt) TokenLiteral() string { return a.Token.Literal }
func (a *AssignStmt) statementNode()       {}

// BlockStmt 代码块
type BlockStmt struct {
	Token      lexer.Token // { token
	Statements []Statement
}

func (b *BlockStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BlockStmt) statementNode()       {}

// ReturnStmt return 语句
type ReturnStmt struct {
	Token  lexer.Token
	Values []Expression
}

func (r *ReturnStmt) TokenLiteral() string { return r.Token.Literal }
func (r *ReturnStmt) statementNode()       {}

// IfStmt if 语句
type IfStmt struct {
	Token       lexer.Token
	Init        Statement  // 初始化语句（可选）
	Condition   Expression // 条件
	Consequence *BlockStmt // then 分支
	Alternative Statement  // else 分支（可选，可以是 BlockStmt 或 IfStmt）
}

func (i *IfStmt) TokenLiteral() string { return i.Token.Literal }
func (i *IfStmt) statementNode()       {}

// ForStmt for 语句
type ForStmt struct {
	Token     lexer.Token
	Init      Statement  // 初始化语句（可选）
	Condition Expression // 条件（可选）
	Post      Statement  // 后置语句（可选）
	Body      *BlockStmt
}

func (f *ForStmt) TokenLiteral() string { return f.Token.Literal }
func (f *ForStmt) statementNode()       {}

// RangeStmt range 循环
type RangeStmt struct {
	Token lexer.Token
	Key   Expression // 键/索引
	Value Expression // 值
	X     Expression // 被迭代对象
	Body  *BlockStmt
}

func (r *RangeStmt) TokenLiteral() string { return r.Token.Literal }
func (r *RangeStmt) statementNode()       {}

// SwitchStmt switch 语句
type SwitchStmt struct {
	Token lexer.Token
	Init  Statement   // 初始化语句
	Tag   Expression  // 标签表达式
	Cases []*CaseClause
}

func (s *SwitchStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SwitchStmt) statementNode()       {}

// CaseClause case 子句
type CaseClause struct {
	Token lexer.Token
	Exprs []Expression // nil 表示 default
	Body  []Statement
}

func (c *CaseClause) TokenLiteral() string { return c.Token.Literal }
func (c *CaseClause) statementNode()       {}

// SelectStmt select 语句
type SelectStmt struct {
	Token lexer.Token
	Cases []*CommClause
}

func (s *SelectStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SelectStmt) statementNode()       {}

// CommClause 通信子句
type CommClause struct {
	Token lexer.Token
	Comm  Statement   // send 或 receive 语句，nil 表示 default
	Body  []Statement
}

func (c *CommClause) TokenLiteral() string { return c.Token.Literal }
func (c *CommClause) statementNode()       {}

// GoStmt go 语句
type GoStmt struct {
	Token lexer.Token
	Call  *CallExpr
}

func (g *GoStmt) TokenLiteral() string { return g.Token.Literal }
func (g *GoStmt) statementNode()       {}

// DeferStmt defer 语句
type DeferStmt struct {
	Token lexer.Token
	Call  *CallExpr
}

func (d *DeferStmt) TokenLiteral() string { return d.Token.Literal }
func (d *DeferStmt) statementNode()       {}

// BreakStmt break 语句
type BreakStmt struct {
	Token lexer.Token
	Label string
}

func (b *BreakStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BreakStmt) statementNode()       {}

// ContinueStmt continue 语句
type ContinueStmt struct {
	Token lexer.Token
	Label string
}

func (c *ContinueStmt) TokenLiteral() string { return c.Token.Literal }
func (c *ContinueStmt) statementNode()       {}

// FallthroughStmt fallthrough 语句
type FallthroughStmt struct {
	Token lexer.Token
}

func (f *FallthroughStmt) TokenLiteral() string { return f.Token.Literal }
func (f *FallthroughStmt) statementNode()       {}

// TryStmt try-catch 语句
type TryStmt struct {
	Token   lexer.Token
	Body    *BlockStmt   // try 块
	Catch   *CatchClause // catch 子句
	Finally *BlockStmt   // finally 子句（可选，后续扩展）
}

func (t *TryStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TryStmt) statementNode()       {}

// CatchClause catch 子句
type CatchClause struct {
	Token lexer.Token
	Param string     // 异常参数名 (e)
	Body  *BlockStmt // catch 块
}

func (c *CatchClause) TokenLiteral() string { return c.Token.Literal }

// ThrowStmt throw 语句
type ThrowStmt struct {
	Token lexer.Token
	Value Expression // 错误值
}

func (t *ThrowStmt) TokenLiteral() string { return t.Token.Literal }
func (t *ThrowStmt) statementNode()       {}

// ExpressionStmt 表达式语句
type ExpressionStmt struct {
	Expression Expression
}

func (e *ExpressionStmt) TokenLiteral() string { return e.Expression.TokenLiteral() }
func (e *ExpressionStmt) statementNode()       {}

// SendStmt 发送语句
type SendStmt struct {
	Token   lexer.Token
	Channel Expression
	Value   Expression
}

func (s *SendStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SendStmt) statementNode()       {}

// IncDecStmt ++ 或 -- 语句
type IncDecStmt struct {
	Token lexer.Token
	X     Expression
	Inc   bool // true: ++, false: --
}

func (i *IncDecStmt) TokenLiteral() string { return i.Token.Literal }
func (i *IncDecStmt) statementNode()       {}

// ========== 表达式 ==========

// Identifier 标识符
type Identifier struct {
	Token lexer.Token
	Value string
}

func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) expressionNode()      {}

// IntegerLiteral 整数字面量
type IntegerLiteral struct {
	Token lexer.Token
	Value string
}

func (i *IntegerLiteral) TokenLiteral() string { return i.Token.Literal }
func (i *IntegerLiteral) expressionNode()      {}

// FloatLiteral 浮点数字面量
type FloatLiteral struct {
	Token lexer.Token
	Value string
}

func (f *FloatLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FloatLiteral) expressionNode()      {}

// StringLiteral 字符串字面量
type StringLiteral struct {
	Token lexer.Token
	Value string
}

func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) expressionNode()      {}

// CharLiteral 字符字面量
type CharLiteral struct {
	Token lexer.Token
	Value string
}

func (c *CharLiteral) TokenLiteral() string { return c.Token.Literal }
func (c *CharLiteral) expressionNode()      {}

// BoolLiteral 布尔字面量
type BoolLiteral struct {
	Token lexer.Token
	Value bool
}

func (b *BoolLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BoolLiteral) expressionNode()      {}

// NilLiteral nil 字面量
type NilLiteral struct {
	Token lexer.Token
}

func (n *NilLiteral) TokenLiteral() string { return n.Token.Literal }
func (n *NilLiteral) expressionNode()      {}

// ArrayLiteral 数组字面量
type ArrayLiteral struct {
	Token    lexer.Token
	Type     Expression   // 元素类型
	Elements []Expression // 元素
}

func (a *ArrayLiteral) TokenLiteral() string { return a.Token.Literal }
func (a *ArrayLiteral) expressionNode()      {}

// SliceLiteral 切片字面量
type SliceLiteral struct {
	Token    lexer.Token
	Type     Expression
	Elements []Expression
}

func (s *SliceLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *SliceLiteral) expressionNode()      {}

// MapLiteral map 字面量
type MapLiteral struct {
	Token   lexer.Token
	KeyType Expression
	ValType Expression
	Pairs   []*KeyValuePair
}

func (m *MapLiteral) TokenLiteral() string { return m.Token.Literal }
func (m *MapLiteral) expressionNode()      {}

// KeyValuePair 键值对
type KeyValuePair struct {
	Key   Expression
	Value Expression
}

// StructLiteral 结构体字面量
type StructLiteral struct {
	Token  lexer.Token
	Type   Expression
	Fields []*FieldValue
}

func (s *StructLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StructLiteral) expressionNode()      {}

// FieldValue 字段值
type FieldValue struct {
	Name  string
	Value Expression
}

// BinaryExpr 二元表达式
type BinaryExpr struct {
	Token    lexer.Token
	Left     Expression
	Operator string
	Right    Expression
}

func (b *BinaryExpr) TokenLiteral() string { return b.Token.Literal }
func (b *BinaryExpr) expressionNode()      {}

// TernaryExpr 三元表达式 (condition ? trueExpr : falseExpr)
type TernaryExpr struct {
	Token     lexer.Token // ? token
	Condition Expression
	TrueExpr  Expression
	FalseExpr Expression
}

func (t *TernaryExpr) TokenLiteral() string { return t.Token.Literal }
func (t *TernaryExpr) expressionNode()      {}

// MatchExpr match 表达式
// match(expr) { pattern => result, ... }
type MatchExpr struct {
	Token   lexer.Token  // match token
	Subject Expression   // 被匹配的表达式
	Arms    []*MatchArm  // 匹配分支
	IsType  bool         // 是否是类型匹配
}

func (m *MatchExpr) TokenLiteral() string { return m.Token.Literal }
func (m *MatchExpr) expressionNode()      {}

// MatchArm 匹配分支
type MatchArm struct {
	Token    lexer.Token  // => token
	Patterns []Expression // 匹配模式（可以是多个值，用逗号分隔）
	IsDefault bool        // 是否是 default 分支
	Body     Expression   // 分支结果表达式
}

// UnaryExpr 一元表达式
type UnaryExpr struct {
	Token    lexer.Token
	Operator string
	Operand  Expression
}

func (u *UnaryExpr) TokenLiteral() string { return u.Token.Literal }
func (u *UnaryExpr) expressionNode()      {}

// CallExpr 函数调用表达式
type CallExpr struct {
	Token     lexer.Token
	Function  Expression   // 被调用的函数
	Arguments []Expression // 参数列表
}

func (c *CallExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CallExpr) expressionNode()      {}

// IndexExpr 索引表达式
type IndexExpr struct {
	Token lexer.Token
	X     Expression // 被索引对象
	Index Expression // 索引
}

func (i *IndexExpr) TokenLiteral() string { return i.Token.Literal }
func (i *IndexExpr) expressionNode()      {}

// SliceExpr 切片表达式
type SliceExpr struct {
	Token lexer.Token
	X     Expression // 被切片对象
	Low   Expression // 起始索引
	High  Expression // 结束索引
	Max   Expression // 最大容量（可选）
}

func (s *SliceExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SliceExpr) expressionNode()      {}

// SelectorExpr 选择器表达式 x.y
type SelectorExpr struct {
	Token lexer.Token
	X     Expression // 对象
	Sel   string     // 选择的成员
}

func (s *SelectorExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SelectorExpr) expressionNode()      {}

// TypeAssertExpr 类型断言表达式
type TypeAssertExpr struct {
	Token lexer.Token
	X     Expression // 被断言对象
	Type  Expression // 断言的类型
}

func (t *TypeAssertExpr) TokenLiteral() string { return t.Token.Literal }
func (t *TypeAssertExpr) expressionNode()      {}

// FuncLiteral 函数字面量（匿名函数）
type FuncLiteral struct {
	Token   lexer.Token
	Params  []*Field
	Results []*Field
	Body    *BlockStmt
}

func (f *FuncLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FuncLiteral) expressionNode()      {}

// ReceiveExpr 接收表达式 <-ch
type ReceiveExpr struct {
	Token lexer.Token
	X     Expression
}

func (r *ReceiveExpr) TokenLiteral() string { return r.Token.Literal }
func (r *ReceiveExpr) expressionNode()      {}

// ========== 类型表达式 ==========

// ArrayType 数组类型
type ArrayType struct {
	Token lexer.Token
	Len   Expression // 长度
	Elt   Expression // 元素类型
}

func (a *ArrayType) TokenLiteral() string { return a.Token.Literal }
func (a *ArrayType) expressionNode()      {}

// SliceType 切片类型
type SliceType struct {
	Token lexer.Token
	Elt   Expression
}

func (s *SliceType) TokenLiteral() string { return s.Token.Literal }
func (s *SliceType) expressionNode()      {}

// MapType map 类型
type MapType struct {
	Token   lexer.Token
	Key     Expression
	Value   Expression
}

func (m *MapType) TokenLiteral() string { return m.Token.Literal }
func (m *MapType) expressionNode()      {}

// ChanType 通道类型
type ChanType struct {
	Token lexer.Token
	Dir   int        // 0: 双向, 1: 只发送, 2: 只接收
	Value Expression // 元素类型
}

func (c *ChanType) TokenLiteral() string { return c.Token.Literal }
func (c *ChanType) expressionNode()      {}

// PointerType 指针类型
type PointerType struct {
	Token lexer.Token
	Base  Expression
}

func (p *PointerType) TokenLiteral() string { return p.Token.Literal }
func (p *PointerType) expressionNode()      {}

// FuncType 函数类型
type FuncType struct {
	Token   lexer.Token
	Params  []*Field
	Results []*Field
}

func (f *FuncType) TokenLiteral() string { return f.Token.Literal }
func (f *FuncType) expressionNode()      {}

// InterfaceType 接口类型（匿名）
type InterfaceType struct {
	Token   lexer.Token
	Methods []*FuncSignature
}

func (i *InterfaceType) TokenLiteral() string { return i.Token.Literal }
func (i *InterfaceType) expressionNode()      {}

// StructType 结构体类型（匿名）
type StructType struct {
	Token  lexer.Token
	Fields []*StructField
}

func (s *StructType) TokenLiteral() string { return s.Token.Literal }
func (s *StructType) expressionNode()      {}

// Ellipsis 可变参数类型 ...T
type Ellipsis struct {
	Token lexer.Token
	Elt   Expression
}

func (e *Ellipsis) TokenLiteral() string { return e.Token.Literal }
func (e *Ellipsis) expressionNode()      {}

// ParenExpr 括号表达式
type ParenExpr struct {
	Token lexer.Token
	X     Expression
}

func (p *ParenExpr) TokenLiteral() string { return p.Token.Literal }
func (p *ParenExpr) expressionNode()      {}

// MakeExpr make 表达式
type MakeExpr struct {
	Token lexer.Token
	Type  Expression
	Args  []Expression
}

func (m *MakeExpr) TokenLiteral() string { return m.Token.Literal }
func (m *MakeExpr) expressionNode()      {}

// NewExpr new 表达式 (new ClassName(args))
type NewExpr struct {
	Token     lexer.Token
	Type      Expression   // 类名
	Arguments []Expression // 构造参数 (命名参数)
}

func (n *NewExpr) TokenLiteral() string { return n.Token.Literal }
func (n *NewExpr) expressionNode()      {}

// LenExpr len 表达式
type LenExpr struct {
	Token lexer.Token
	X     Expression
}

func (l *LenExpr) TokenLiteral() string { return l.Token.Literal }
func (l *LenExpr) expressionNode()      {}

// CapExpr cap 表达式
type CapExpr struct {
	Token lexer.Token
	X     Expression
}

func (c *CapExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CapExpr) expressionNode()      {}

// AppendExpr append 表达式
type AppendExpr struct {
	Token lexer.Token
	Slice Expression
	Elems []Expression
}

func (a *AppendExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AppendExpr) expressionNode()      {}

// CopyExpr copy 表达式
type CopyExpr struct {
	Token lexer.Token
	Dst   Expression
	Src   Expression
}

func (c *CopyExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CopyExpr) expressionNode()      {}

// DeleteExpr delete 表达式
type DeleteExpr struct {
	Token lexer.Token
	Map   Expression
	Key   Expression
}

func (d *DeleteExpr) TokenLiteral() string { return d.Token.Literal }
func (d *DeleteExpr) expressionNode()      {}

// RawCode 原始代码（直接透传到 Go）
type RawCode struct {
	Token lexer.Token
	Code  string
}

func (r *RawCode) TokenLiteral() string { return r.Token.Literal }
func (r *RawCode) statementNode()       {}
func (r *RawCode) expressionNode()      {}

// ========== 泛型相关 ==========

// TypeParam 类型参数 (如 T any, K comparable)
type TypeParam struct {
	Name       string     // 类型参数名 (如 T, K)
	Constraint Expression // 类型约束 (如 any, comparable, int|string)
}

// TypeParamList 类型参数列表 [T any, K comparable]
type TypeParamList struct {
	Token  lexer.Token
	Params []*TypeParam
}

func (t *TypeParamList) TokenLiteral() string { return t.Token.Literal }

// GenericType 泛型类型实例化 (如 List[int], Map[string, int])
type GenericType struct {
	Token    lexer.Token
	Type     Expression   // 基础类型名 (如 List, Map)
	TypeArgs []Expression // 类型参数 (如 int, string)
}

func (g *GenericType) TokenLiteral() string { return g.Token.Literal }
func (g *GenericType) expressionNode()      {}

// UnionType 联合类型约束 (如 int | string | float64)
type UnionType struct {
	Token lexer.Token
	Types []Expression
}

func (u *UnionType) TokenLiteral() string { return u.Token.Literal }
func (u *UnionType) expressionNode()      {}
