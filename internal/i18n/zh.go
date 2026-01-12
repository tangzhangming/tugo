package i18n

// zhMessages contains Chinese translations
var zhMessages = map[string]string{
	// Parser errors
	ErrExpectedToken: "第 %d 行第 %d 列: 期望 %s, 实际是 %s",
	ErrGeneric:       "第 %d 行第 %d 列: %s",

	// Interface implementation errors
	ErrInterfaceNotFound:       "类 %s: 接口 %s 未找到",
	ErrMissingMethod:           "类 %s 未实现接口 %s: 缺少方法 %s",
	ErrParamCountMismatch:      "类 %s 方法 %s: 参数数量不匹配 (实际 %d 个, 接口 %s 要求 %d 个)",
	ErrReturnCountMismatch:     "类 %s 方法 %s: 返回值数量不匹配 (实际 %d 个, 接口 %s 要求 %d 个)",
	ErrStructInterfaceNotFound: "结构体 %s: 接口 %s 未找到",
	ErrStructMissingMethod:     "结构体 %s 未实现接口 %s: 缺少方法 %s",
	ErrStructParamMismatch:     "结构体 %s 方法 %s: 参数数量不匹配 (实际 %d 个, 接口 %s 要求 %d 个)",
	ErrStructReturnMismatch:    "结构体 %s 方法 %s: 返回值数量不匹配 (实际 %d 个, 接口 %s 要求 %d 个)",

	// Extends/inheritance errors
	ErrParentClassNotFound:    "类 %s: 父类 %s 未找到",
	ErrExtendNonAbstract:      "类 %s: 不能继承非抽象类 %s",
	ErrAbstractMethodMissing:  "类 %s 未实现父类 %s 的抽象方法 %s",
	ErrAbstractParamMismatch:  "类 %s 方法 %s: 参数数量不匹配 (实际 %d 个, 父类 %s 抽象方法要求 %d 个)",
	ErrAbstractReturnMismatch: "类 %s 方法 %s: 返回值数量不匹配 (实际 %d 个, 父类 %s 抽象方法要求 %d 个)",

	// Static class errors
	ErrStaticClassInit: "静态类 %s 不能有 init 构造函数",
	ErrStaticClassThis: "静态类 %s 的方法 %s 不能使用 'this', 请使用 'self::' 代替",

	// Top-level statement errors
	ErrTopLevelFunction: "函数必须定义在类内部, 发现顶层函数 '%s'",
	ErrTopLevelVariable: "变量必须定义在类内部, 发现顶层变量 '%s'",
	ErrTopLevelConstant: "常量必须定义在类内部, 发现顶层常量 '%s'",

	// File naming errors
	ErrTooManyPublicTypes:  "文件 '%s' 有 %d 个公开类/接口, 只允许有一个",
	ErrPublicClassFileName: "公开类 '%s' 必须在文件 '%s.tugo' 中, 但发现在 '%s.tugo' 中",
	ErrPublicIfaceFileName: "公开接口 '%s' 必须在文件 '%s.tugo' 中, 但发现在 '%s.tugo' 中",

	// Main method errors
	ErrMainNotStatic:  "类 '%s' 的 main 方法必须是 static",
	ErrMainNotPublic:  "类 '%s' 的 main 方法必须是 public",
	ErrMainHasParams:  "类 '%s' 的 main 方法不能有参数",
	ErrMainHasReturns: "类 '%s' 的 main 方法不能有返回值",

	// Errable function errors
	ErrErrableNotHandled:       "函数 %s: 调用可能出错的函数 '%s' 必须在 try 块内，或者当前函数必须标记为 errable",
	ErrErrableMethodNotHandled: "函数 %s: 调用可能出错的方法 '%s' 必须在 try 块内，或者当前函数必须标记为 errable",

	// Symbol errors
	ErrUndefinedType: "未定义的类型 '%s': 未导入或未定义",
	ErrUnusedImport:  "导入的类型 '%s' 未被使用 (来自 %s)",

	// Codegen errors
	ErrTooManyVariables:           "变量太多: 函数返回 %d 个值，但尝试赋值给 %d 个变量",
	ErrErrableMultiReturnNoAssign: "返回 %d 个值的 errable 函数不能直接作为表达式使用，必须赋值给变量",

	// CLI - Usage and help
	MsgUsage:          "用法: tugo <命令> [参数]",
	MsgCommands:       "命令:",
	MsgCmdRun:         "  run      转译并运行 tugo 源文件",
	MsgCmdBuild:       "  build    将 tugo 源文件转译为 Go",
	MsgCmdVersion:     "  version  打印版本信息",
	MsgCmdHelp:        "  help     打印帮助信息",
	MsgUseHelp:        "使用 \"tugo <命令> -h\" 获取命令的更多信息。",
	MsgUnknownCommand: "未知命令: %s",

	// CLI - Run command
	MsgRunUsage:       "用法: tugo run [选项] <输入>",
	MsgRunDescription: "转译 tugo 源文件到 Go 并运行。\n输出放在 .output 目录（自动清理）。",
	MsgRunArgInput:    "  <输入>    输入文件或目录",
	MsgRunOptVerbose:  "详细输出",

	// CLI - Build command
	MsgBuildUsage:       "用法: tugo build [选项] <输入>",
	MsgBuildDescription: "将 tugo 源文件转译为 Go。",
	MsgBuildArgInput:    "  <输入>    输入文件或目录",
	MsgBuildOptOutput:   "输出目录",
	MsgBuildOptVerbose:  "详细输出",
	MsgBuildCompleted:   "构建完成: %s",
	MsgBuildCompletedV:  "构建完成。输出: %s",

	// CLI - Common errors
	ErrInputRequired:     "错误: 需要输入文件或目录",
	ErrCannotGetCwd:      "错误: 无法获取当前目录: %v",
	ErrCannotCleanDir:    "错误: 无法清理输出目录: %v",
	ErrCannotAccessInput: "无法访问输入",
	ErrCannotLoadConfig:  "无法加载配置",
	ErrCannotReadFile:    "无法读取文件",
	ErrParseError:        "%s 解析错误: %s",
	ErrTranspileError:    "%s 转译错误",
	ErrNoTugoFiles:       "在 %s 中未找到 .tugo 文件",
	ErrCannotCreateDir:   "无法创建输出目录",
	ErrCannotWriteFile:   "无法写入文件",
	ErrCannotWriteGoMod:  "无法写入 go.mod",
	ErrRunError:          "运行错误: %v",

	// CLI - Info messages
	MsgUsingConfig:      "使用配置: %s (模块: %s)",
	MsgNoConfig:         "未找到 tugo.toml，使用默认模块: %s",
	MsgParsing:          "正在解析: %s",
	MsgTranspiling:      "正在转译: %s -> %s",
	MsgRunning:          "正在运行...",
	MsgTranspileSuccess: "成功转译 %d 个文件",
	MsgGeneratedGoMod:   "已生成 go.mod，模块: %s",

	// CLI - Stdlib messages
	MsgStdlibNotFound:    "标准库目录未找到: %s",
	MsgStdlibPkgNotFound: "警告: 标准库包未找到: %s",
	MsgParsingStdlib:     "正在解析标准库: %s",
	MsgTranspilingStdlib: "正在转译标准库: %s -> %s",
	MsgCopyingStdlib:     "正在复制标准库: %s -> %s",
	ErrStdlibReadError:   "无法读取标准库文件",
	ErrStdlibParseError:  "标准库 %s 解析错误: %s",
	ErrStdlibWriteError:  "无法写入标准库文件",
	ErrStdlibTranspile:   "无法转译标准库",
}
