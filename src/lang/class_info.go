package lang

// ClassInfo 类型信息结构体
// 用于在运行时获取 tugo class 的元信息
type ClassInfo struct {
	Name     string     // 类名，如 "User"
	Package  string     // 包名，如 "main"
	FullName string     // 完整名，如 "main.User"
	Parent   *ClassInfo // 父类信息（如果有 extends）
}

// String 返回类的完整名称
func (c *ClassInfo) String() string {
	return c.FullName
}

// IsChildOf 判断是否是指定类的子类
func (c *ClassInfo) IsChildOf(parent *ClassInfo) bool {
	if c.Parent == nil {
		return false
	}
	if c.Parent == parent {
		return true
	}
	return c.Parent.IsChildOf(parent)
}
