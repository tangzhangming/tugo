# Tugo 语法文档

Tugo 是一种转译到 Go 的编程语言。本文档仅描述 Tugo 独有的语法特性，与 Go 兼容的语法请参考 [Go 语言规范](https://go.dev/ref/spec)。

---

## 1. 可见性修饰符

Tugo 使用 `public` 关键字声明公开成员，未标记的成员默认私有。

### 语法

```tugo
// 公开函数 -> 翻译为 Go 首字母大写
public func MyFunc() {}

// 私有函数 -> 翻译为 Go 首字母小写
func myFunc() {}

// 公开结构体
public struct User {}

// 公开接口
public interface Reader {}
```

### 翻译规则

| Tugo | Go |
|------|-----|
| `public func test()` | `func Test()` |
| `func test()` | `func test()` |
| `public struct User` | `type User struct` |
| `struct user` | `type user struct` |

---

## 2. 函数默认参数

Tugo 函数支持默认参数值，翻译时生成 Options 结构体模式。

### 语法

```tugo
public func greet(name:string="World", times:int=1) {
    println("Hello, " + name)
}
```

### 调用方式

```tugo
greet()                    // 使用全部默认值
greet("Alice")             // name="Alice", times=1
greet("Bob", 3)            // name="Bob", times=3
```

### 翻译结果

```go
type GreetOpts struct {
    Name  string
    Times int
}

func NewDefaultGreetOpts() GreetOpts {
    return GreetOpts{
        Name:  "World",
        Times: 1,
    }
}

func Greet(opts GreetOpts) {
    name := opts.Name
    times := opts.Times
    fmt.Println("Hello, " + name)
}

// 调用翻译
Greet(GreetOpts{Name: "World", Times: 1})
Greet(GreetOpts{Name: "Alice", Times: 1})
Greet(GreetOpts{Name: "Bob", Times: 3})
```

---

## 3. $ 变量名

Tugo 允许变量名以 `$` 开头，翻译时转换为 `_tugo_` 前缀。

### 语法

```tugo
$name := "test"
var $count int = 10
```

### 翻译结果

```go
_tugo_name := "test"
var _tugo_count int = 10
```

---

## 4. 全局函数

Tugo 提供简化的打印函数，自动导入 `fmt` 包。

| Tugo | Go |
|------|-----|
| `print(x)` | `fmt.Print(x)` |
| `println(x)` | `fmt.Println(x)` |
| `print_f(format, args...)` | `fmt.Printf(format, args...)` |

### 示例

```tugo
print("no newline")
println("with newline")
print_f("Name: %s, Age: %d\n", name, age)
```

---

## 5. Struct 结构体

Tugo 结构体支持构造方法、内部方法和嵌入，但不支持继承。

### 基本语法

```tugo
public struct Point {
    public x int
    public y int
    private label string
    
    // 构造函数（支持默认参数）
    public func init(x:int=0, y:int=0) {
        this.x = x
        this.y = y
        this.label = "point"
    }
    
    // 公开方法
    public func GetX() int {
        return this.x
    }
    
    // 私有方法
    func getLabel() string {
        return this.label
    }
}
```

### 嵌入（组合）

```tugo
public struct ColorPoint {
    Point           // 嵌入 Point（匿名字段）
    public color string
    
    public func init(color:string="red") {
        this.color = color
    }
}
```

### 接口实现

```tugo
public interface Shape {
    Area() float64
}

public struct Circle implements Shape {
    public radius float64
    
    public func Area() float64 {
        return 3.14159 * this.radius * this.radius
    }
}
```

### 翻译规则

| Tugo | Go |
|------|-----|
| `public struct Point` | `type Point struct` |
| `func init(...)` | `func NewPoint(opts) *Point` (指针返回) |
| `public func Method()` | `func (s *Point) Method()` (指针接收者) |
| `this.x` | `s.X` (接收者 + 字段名转换) |
| `Point` (嵌入) | `Point` (Go 匿名字段) |

### struct 与 class 的区别

| 特性 | struct | class |
|------|--------|-------|
| 继承 (`extends`) | **不支持** | 支持 |
| 接口 (`implements`) | 支持 | 支持 |
| 嵌入（组合） | **支持** | 不支持 |
| 静态成员 | **不支持** | 支持 |

### 翻译示例

```go
type Point struct {
    X     int
    Y     int
    label string
}

type PointInitOpts struct {
    X int
    Y int
}

func NewDefaultPointInitOpts() PointInitOpts {
    return PointInitOpts{X: 0, Y: 0}
}

func NewPoint(opts PointInitOpts) *Point {
    s := &Point{}
    s.X = opts.X
    s.Y = opts.Y
    s.label = "point"
    return s
}

func (s *Point) GetX() int {
    return s.X
}
```

---

## 6. Class 类

Tugo 支持面向对象的 class 语法，翻译为 Go 结构体和方法。

### 基本语法

```tugo
public class Person {
    // 静态私有字段 -> 包级变量
    private static string species = "Human"
    
    // 实例字段
    var name string
    var age int = 18              // 带默认值
    public var email string       // 公开字段
    
    // 构造函数
    func init(name:string="Anonymous", age:int=18) {
        this.name = name
        this.age = age
    }
    
    // 公开方法
    public func Greet() {
        println("Hello, I am " + this.name)
    }
    
    // 私有方法
    func getAge() int {
        return this.age
    }
}
```

### 翻译规则

| Tugo | Go |
|------|-----|
| `class Person` | `type Person struct` |
| `func init(...)` | `func NewPerson(opts) *Person` |
| `this.name` | `p.name` (接收者) |
| `private static var` | 包级私有变量 |
| `public func Method()` | `func (p *Person) Method()` |

### 实例化

```tugo
p1 := Person.init()                      // 默认参数
p2 := Person.init(name: "Alice")         // 部分参数
p3 := Person.init(name: "Bob", age: 25)  // 全部参数
```

### 翻译结果示例

```go
// 静态字段
var _person_species string = "Human"

// 结构体
type Person struct {
    name  string
    age   int
    Email string
}

// 默认参数
type NewPersonOpts struct {
    Name string
    Age  int
}

func NewDefaultNewPersonOpts() NewPersonOpts {
    return NewPersonOpts{
        Name: "Anonymous",
        Age:  18,
    }
}

// 构造函数
func NewPerson(opts NewPersonOpts) *Person {
    name := opts.Name
    age := opts.Age
    p := &Person{}
    p.name = name
    p.age = age
    return p
}

// 方法
func (p *Person) Greet() {
    fmt.Println("Hello, I am " + p.name)
}

func (p *Person) getAge() int {
    return p.age
}
```

---

## 7. Interface 实现 (implements)

Tugo 支持 OOP 风格的接口实现声明，使用 `implements` 关键字。

### 语法

```tugo
public interface Reader {
    Read(p []byte) (int, error)
}

public interface Writer {
    Write(p []byte) (int, error)
}

public class FileHandler implements Reader, Writer {
    public func Read(p []byte) (int, error) {
        // 实现
    }
    
    public func Write(p []byte) (int, error) {
        // 实现
    }
}
```

### 行为

1. **编译时校验**：转译器检查类是否实现了接口的所有方法
2. **签名匹配**：方法参数数量、返回值数量必须匹配
3. **隐式实现**：生成的 Go 代码不包含显式声明（Go 接口是隐式实现的）

### 错误示例

```tugo
public interface Greeter {
    Greet() string
}

// 错误：缺少 Greet 方法
public class BadImpl implements Greeter {
    // 转译错误：class BadImpl does not implement interface Greeter: missing method Greet
}

// 错误：方法签名不匹配
public class BadImpl2 implements Greeter {
    public func Greet() {  // 缺少返回值
        println("Hi")
    }
    // 转译错误：return value count mismatch
}
```

### 翻译结果

```go
type Reader interface {
    Read(p []byte) (int, error)
}

type Writer interface {
    Write(p []byte) (int, error)
}

type FileHandler struct {
    // ...
}

func (f *FileHandler) Read(p []byte) (int, error) {
    // 实现
}

func (f *FileHandler) Write(p []byte) (int, error) {
    // 实现
}
// 注意：Go 代码无需声明 implements，由编译器隐式检查
```

---

## 8. 字段可见性修饰符

在 class 和 struct 中可以使用可见性修饰符。

### 语法

```tugo
public class Example {
    var privateField string           // 默认私有
    private var explicitPrivate int   // 显式私有
    public var publicField bool       // 公开
    protected var protectedField int  // 受保护（当前等同于私有）
}
```

### 翻译规则

| 修饰符 | Go 字段名 |
|--------|-----------|
| 无/private | 首字母小写 |
| public | 首字母大写 |
| protected | 首字母小写 |

---

## 9. 抽象类 (abstract class)

Tugo 支持抽象类，使用 `abstract` 关键字声明。抽象类不能直接实例化，必须被子类继承。

### 语法

```tugo
// 定义抽象类
public abstract class Animal {
    var name string
    
    // 具体方法（有实现）
    func getName() string {
        return this.name
    }
    
    // 抽象方法（没有方法体）
    abstract func speak() string
    abstract func move()
}

// 子类继承抽象类
public class Dog extends Animal {
    func init(name:string="Buddy") {
        this.name = name
    }
    
    // 必须实现所有抽象方法
    public func speak() string {
        return "Woof!"
    }
    
    public func move() {
        println("Running...")
    }
}
```

### 翻译规则

抽象类翻译为 **接口** + **基础结构体**：

| Tugo | Go |
|------|-----|
| `abstract class Animal` | `type Animal interface` + `type animalBase struct` |
| `abstract func speak()` | 接口方法 |
| `func getName()` | 基础结构体方法 |
| `class Dog extends Animal` | `type Dog struct { animalBase }` |

### 翻译结果示例

```go
// 抽象方法 -> 接口
type Animal interface {
    Speak() string
    Move()
}

// 字段和具体方法 -> 基础结构体
type animalBase struct {
    name string
}

func (a *animalBase) getName() string {
    return a.name
}

// 子类嵌入基础结构体
type Dog struct {
    animalBase
}

func NewDog(opts DogInitOpts) *Dog {
    t := &Dog{}
    t.name = opts.Name
    return t
}

func (t *Dog) Speak() string {
    return "Woof!"
}

func (t *Dog) Move() {
    fmt.Println("Running...")
}
```

### 校验规则

1. **抽象类不能直接实例化**
2. **子类必须实现所有抽象方法**（除非子类也是抽象类）
3. **方法签名必须匹配**：参数数量和返回值数量必须一致

### 错误示例

```tugo
public abstract class Shape {
    abstract func area() float64
}

// 错误：缺少 area 方法实现
public class BadSquare extends Shape {
    var side float64
    // 转译错误：class BadSquare does not implement abstract method area from parent class Shape
}
```

---

## 10. 静态类 (static class)

Tugo 支持静态类，类似于 C# 的静态类。静态类不能被实例化，所有成员都是静态的。

### 特性

- **不能被实例化**：没有构造函数 (`init`)
- **不能被继承**：其他类不能 `extends` 静态类
- **所有成员隐式为静态**：不需要显式写 `static`
- **没有 `this`**：使用 `self::` 访问自身成员

### 语法

```tugo
public static class Helper {
    public var name string = "default"
    private var count int = 0
    
    public func greet() {
        println("Hello, I am " + self::name)
    }
    
    func increment() {
        self::count = self::count + 1
    }
    
    public func getCount() int {
        return self::count
    }
}
```

### 访问方式

```tugo
// 外部访问使用 ClassName::member
Helper::greet()
Helper::name = "Tugo"
count := Helper::getCount()

// 类内部访问使用 self::member
self::name
self::greet()
```

### 翻译规则

静态类翻译为包级变量和函数：

| Tugo | Go |
|------|-----|
| `public var name` | `HelperName` (包级变量) |
| `private var count` | `_helper_count` (包级变量) |
| `public func greet()` | `HelperGreet()` (包级函数) |
| `func increment()` | `helperIncrement()` (包级函数) |
| `self::name` | `HelperName` |
| `Helper::greet()` | `HelperGreet()` |

### 翻译结果示例

```go
var HelperName string = "default"
var _helper_count int = 0

func HelperGreet() {
    fmt.Println("Hello, I am " + HelperName)
}

func helperIncrement() {
    _helper_count = _helper_count + 1
}

func HelperGetCount() int {
    return _helper_count
}

// 调用翻译
HelperGreet()
HelperName = "Tugo"
```

### 校验规则

1. **静态类不能有 init 构造函数**
2. **静态类不能使用 this**：必须使用 `self::` 访问成员
3. **静态类不能被继承**：不能作为 `extends` 的目标
4. **静态类不能实现接口**：不能使用 `implements`

### 错误示例

```tugo
// 错误：静态类不能有构造函数
public static class Bad1 {
    func init() {}
    // 转译错误：static class Bad1 cannot have init constructor
}

// 错误：静态类不能使用 this
public static class Bad2 {
    var name string
    public func test() {
        println(this.name)  // 错误
    }
    // 转译错误：static class Bad2 method test cannot use 'this', use 'self::' instead
}
```

---

## 11. 包管理与导入

Tugo 采用 Java 风格的包管理系统，支持精确的类型导入。详细文档请参考 [包管理](包管理.md)。

### 项目配置 (tugo.toml)

```toml
[project]
module = "com.company.demo"
```

### 导入语法

```tugo
// 导入 tugo 包中的类型
import "com.company.demo.models.User"
import "com.company.demo.models.Order"

// 使用别名
import UserModel "com.company.demo.models.User"

// 导入 Go 标准库
import "fmt" from golang
import "strings" from golang

// 导入 tugo 标准库
import "tugo.lang.Str"
```

### 翻译规则

| Tugo | Go |
|------|-----|
| `import "com.example.models.User"` | `import "com.example/models"` |
| `import "fmt" from golang` | `import "fmt"` |
| `import "tugo.lang.Str"` | `import "tugo/lang"` |
| `User.Method()` | `models.User.Method()` |

---

## 关键字总览

| 关键字 | 用途 |
|--------|------|
| `public` | 声明公开成员 |
| `private` | 声明私有成员 |
| `protected` | 声明受保护成员 |
| `struct` | 定义结构体 |
| `class` | 定义类 |
| `abstract` | 声明抽象类或抽象方法 |
| `extends` | 继承父类（仅 class） |
| `static` | 声明静态成员/静态类 |
| `this` | 引用当前实例 |
| `self` | 引用静态类自身（配合 `::` 使用）|
| `implements` | 声明接口实现（class 和 struct）|
| `::` | 静态成员访问运算符 |
| `from` | 指定导入来源 (与 `import` 配合) |
