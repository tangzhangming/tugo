# Tugo 表达式语法

## 三元运算符

Tugo 支持 C 风格的三元运算符，语法为 `condition ? trueValue : falseValue`。

### 基本用法

```tugo
// 简单赋值
a := 10
b := a > 5 ? "大于5" : "小于等于5"

// 作为函数参数
println(a > 0 ? "正数" : "非正数")

// 数值类型
x := 3
y := 7
max := x > y ? x : y
```

### 嵌套三元运算符

```tugo
score := 85
grade := score >= 90 ? "A" : (score >= 80 ? "B" : (score >= 70 ? "C" : "D"))
```

### 类型检查

三元运算符的两个分支**必须类型一致**，否则编译时报错：

```tugo
// ❌ 错误：类型不匹配
result := flag ? "string" : 123

// ✅ 正确：类型一致
result := flag ? "yes" : "no"
```

### 生成的 Go 代码

```tugo
max := x > y ? x : y
```

翻译为：

```go
max := func() int { if x > y { return x }; return y }()
```

---

## Match 模式匹配

Tugo 支持强大的 `match` 表达式，用于值匹配。

### 基本语法

```tugo
result := match(expr) {
    pattern1 => value1,
    pattern2 => value2,
    default => defaultValue
}
```

### 简单值匹配

```tugo
httpCode := 200
message := match(httpCode) {
    200 => "OK",
    404 => "Not Found",
    500 => "Server Error",
    default => "Unknown"
}
```

### 多值匹配

一个分支可以匹配多个值，用逗号分隔：

```tugo
code := 401
status := match(code) {
    200, 0 => "成功",
    400, 401, 403 => "客户端错误",
    500, 502, 503 => "服务器错误",
    default => "未知状态"
}
```

### 表达式作为匹配条件

```tugo
score := 85
grade := match(score / 10) {
    10, 9 => "A",
    8 => "B",
    7 => "C",
    6 => "D",
    default => "F"
}
```

### 字符串匹配

```tugo
color := "red"
hex := match(color) {
    "red" => "#FF0000",
    "green" => "#00FF00",
    "blue" => "#0000FF",
    default => "#000000"
}
```

### 作为函数参数

```tugo
println("状态:", match(code) {
    200 => "成功",
    404 => "未找到",
    default => "其他"
})
```

### 生成的 Go 代码

`match` 表达式使用临时变量 + `switch` 语句，避免闭包开销：

```tugo
message := match(httpCode) {
    200 => "OK",
    404 => "Not Found",
    default => "Unknown"
}
```

翻译为：

```go
var __match_1 string
switch httpCode {
case 200:
    __match_1 = "OK"
case 404:
    __match_1 = "Not Found"
default:
    __match_1 = "Unknown"
}
message := __match_1
```

---

## 对比表

| 特性 | 三元运算符 | Match 表达式 |
|------|-----------|-------------|
| 语法 | `cond ? a : b` | `match(x) { ... }` |
| 分支数 | 2 | 无限制 |
| 多值匹配 | ❌ | ✅ |
| default | 必须有 false 分支 | 可选 |
| 适用场景 | 简单二选一 | 复杂多分支 |

---

## 最佳实践

1. **简单二选一** → 使用三元运算符
2. **多分支匹配** → 使用 match
3. **多值共用结果** → 使用 match 的多值语法
4. **始终提供 default** → 避免意外情况
