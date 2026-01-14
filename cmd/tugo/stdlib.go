package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/i18n"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
	"github.com/tangzhangming/tugo/internal/transpiler"
)

// 标准库配置常量
const (
	stdlibRelDir    = "src"       // 标准库相对 tugo.exe 的目录
	stdlibCoreDir   = "core"      // tugo 代码目录
	stdlibRuntimeDir = "runtime"  // go 代码目录（直接复制）
	stdlibPkgPrefix = "tugo."     // 标准库包前缀
)

// getStdlibDir 获取标准库目录（tugo.exe 同级的 src/ 目录）
func getStdlibDir() (string, error) {
	// 获取当前可执行文件路径
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	stdlibDir := filepath.Join(exeDir, stdlibRelDir)

	// 检查目录是否存在
	if _, err := os.Stat(stdlibDir); os.IsNotExist(err) {
		// 尝试从当前工作目录查找（用于开发环境）
		cwd, err := os.Getwd()
		if err == nil {
			stdlibDir = filepath.Join(cwd, stdlibRelDir)
			if _, err := os.Stat(stdlibDir); err == nil {
				return stdlibDir, nil
			}
		}
		return "", &stdlibError{path: stdlibDir}
	}

	return stdlibDir, nil
}

// stdlibError 标准库目录不存在错误
type stdlibError struct {
	path string
}

func (e *stdlibError) Error() string {
	return i18n.T(i18n.MsgStdlibNotFound, e.path)
}

// collectTugoImports 收集文件中的 tugo 标准库导入
func collectTugoImports(files []*parser.File) map[string]bool {
	imports := make(map[string]bool)
	for _, file := range files {
		// 收集显式导入
		for _, imp := range file.Imports {
			for _, spec := range imp.Specs {
				// 检查是否是 tugo 标准库导入（use 语句且路径以 tugo. 开头）
				if !spec.IsGoImport && len(spec.Path) >= 5 && spec.Path[:5] == "tugo." {
					// tugo.lang.Str -> tugo.lang
					imports[spec.PkgPath] = true
				}
			}
		}
		
		// 检测是否有非静态类声明（需要隐式导入 tugo.runtime 用于 ClassInfo）
		for _, stmt := range file.Statements {
			if classDecl, ok := stmt.(*parser.ClassDecl); ok {
				if !classDecl.Static {
					imports["tugo.runtime"] = true
					break // 只需要检测到一个就够了
				}
			}
		}
	}
	return imports
}

// preloadStdlibClasses 预解析标准库类信息
// 在转译用户代码之前解析标准库文件，以便用户代码可以获取父类的方法信息
func preloadStdlibClasses(stdlibDir string, tugoImports map[string]bool, verbose bool) []*parser.File {
	var result []*parser.File
	coreDir := filepath.Join(stdlibDir, stdlibCoreDir)

	for pkgPath := range tugoImports {
		// 跳过 runtime 包（纯 Go 代码）
		if pkgPath == "tugo.runtime" {
			continue
		}

		// 去掉 tugo. 前缀获取相对路径
		srcRelPath := pkgPath
		if strings.HasPrefix(srcRelPath, stdlibPkgPrefix) {
			srcRelPath = srcRelPath[len(stdlibPkgPrefix):]
		}
		srcRelPath = strings.ReplaceAll(srcRelPath, ".", string(filepath.Separator))

		srcDir := filepath.Join(coreDir, srcRelPath)

		// 检查目录是否存在
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}

		// 读取目录内容
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			continue
		}

		// 解析所有 .tugo 文件
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tugo") {
				continue
			}
			
			// TODO: 暂时跳过有泛型静态方法问题的文件
			if entry.Name() == "query_builder.tugo" {
				continue
			}

			srcFile := filepath.Join(srcDir, entry.Name())
			source, err := os.ReadFile(srcFile)
			if err != nil {
				if verbose {
					printWarning(fmt.Sprintf("读取标准库文件失败 %s: %v", srcFile, err))
				}
				continue
			}

			file, errors := parser.Parse(string(source))
			if len(errors) > 0 {
				if verbose {
					printWarning(fmt.Sprintf("解析标准库文件 %s 失败: %s（继续处理）", srcFile, errors[0]))
				}
				continue
			}

			result = append(result, file)

			if verbose {
				printInfo(fmt.Sprintf("预加载标准库: %s", srcFile))
			}
		}
	}

	return result
}

// transpileStdlib 转译标准库到 vendor 目录
func transpileStdlib(stdlibDir, outputDir string, tugoImports map[string]bool, verbose bool, cfg *config.Config) error {
	if len(tugoImports) == 0 {
		return nil
	}

	coreDir := filepath.Join(stdlibDir, stdlibCoreDir)
	runtimeDir := filepath.Join(stdlibDir, stdlibRuntimeDir)

	for pkgPath := range tugoImports {
		// 去掉 tugo. 前缀获取相对路径
		srcRelPath := pkgPath
		if strings.HasPrefix(srcRelPath, stdlibPkgPrefix) {
			srcRelPath = srcRelPath[len(stdlibPkgPrefix):] // 去掉前缀
		}
		srcRelPath = strings.ReplaceAll(srcRelPath, ".", string(filepath.Separator))

		// 输出目录: tugo/db/ 或 tugo/runtime/（直接在项目根目录下）
		outPkgDir := filepath.Join(outputDir, strings.ReplaceAll(pkgPath, ".", "/"))
		if err := os.MkdirAll(outPkgDir, 0755); err != nil {
			return &createDirError{path: outPkgDir, err: err}
		}

		// 检查是否是 runtime 包（直接复制 .go 文件）
		if srcRelPath == "runtime" {
			if err := copyRuntimeFiles(runtimeDir, outPkgDir, verbose); err != nil {
				return err
			}
			continue
		}

		// 其他包从 core 目录编译
		srcDir := filepath.Join(coreDir, srcRelPath)

		// 检查源目录是否存在
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			if verbose {
				printInfo(i18n.T(i18n.MsgStdlibPkgNotFound, pkgPath))
			}
			continue
		}

		// 读取目录内容
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			return &stdlibReadDirError{path: srcDir, err: err}
		}

		var pkgFiles []*parser.File
		var fileNames []string

		// 第一遍：解析所有 .tugo 文件
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tugo") {
				continue
			}
			
			// TODO: 暂时跳过有泛型静态方法问题的文件
			if entry.Name() == "query_builder.tugo" {
				if verbose {
					printWarning("跳过 query_builder.tugo（泛型静态方法暂不支持）")
				}
				continue
			}

			srcFile := filepath.Join(srcDir, entry.Name())
			source, err := os.ReadFile(srcFile)
			if err != nil {
				printWarning(fmt.Sprintf("读取标准库文件失败 %s: %v", srcFile, err))
				continue
			}

			file, errors := parser.Parse(string(source))
			if len(errors) > 0 {
				printWarning(fmt.Sprintf("解析标准库文件 %s 失败: %s（跳过）", srcFile, errors[0]))
				continue
			}

			pkgFiles = append(pkgFiles, file)
			fileNames = append(fileNames, strings.TrimSuffix(entry.Name(), ".tugo"))

			if verbose {
				printInfo(i18n.T(i18n.MsgParsingStdlib, srcFile))
			}
		}

		// 复制所有 .go 文件（预编译的 Go 代码）
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}

			srcFile := filepath.Join(srcDir, entry.Name())
			source, err := os.ReadFile(srcFile)
			if err != nil {
				return &stdlibReadFileError{path: srcFile, err: err}
			}

			outFile := filepath.Join(outPkgDir, entry.Name())
			if err := os.WriteFile(outFile, source, 0644); err != nil {
				return &stdlibWriteError{path: outFile, err: err}
			}

			if verbose {
				printInfo(i18n.T(i18n.MsgCopyingStdlib, entry.Name(), outFile))
			}
		}

		if len(pkgFiles) == 0 {
			// 没有 .tugo 文件，只有 .go 文件（已复制完成）
			continue
		}

		// 构建符号表
		table := symbol.Collect(pkgFiles)

		// 第二遍：转译（标准库跳过验证）
		t := transpiler.New(table)
		t.SetConfig(cfg)
		t.SetSkipValidation(true) // 标准库跳过顶层语句验证

		for i, file := range pkgFiles {
			goCode, err := t.TranspileFileWithName(file, fileNames[i])
			if err != nil {
				return &stdlibTranspileError{err: err}
			}

			outFile := filepath.Join(outPkgDir, fileNames[i]+".go")
			if err := os.WriteFile(outFile, []byte(goCode), 0644); err != nil {
				return &stdlibWriteError{path: outFile, err: err}
			}

			if verbose {
				printInfo(i18n.T(i18n.MsgTranspilingStdlib, fileNames[i]+".tugo", outFile))
			}
		}

		// 为标准库子包生成 go.mod 文件
		goPkgPath := strings.ReplaceAll(pkgPath, ".", "/")
		goModPath := filepath.Join(outPkgDir, "go.mod")
		goModContent := generateStdlibGoMod(goPkgPath, pkgPath == "tugo.db")
		if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
			return &stdlibWriteError{path: goModPath, err: err}
		}
	}

	return nil
}

// generateStdlibGoMod 生成标准库子包的 go.mod 文件
func generateStdlibGoMod(modulePath string, needsGorm bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("module %s\n\ngo 1.21\n", modulePath))

	if needsGorm {
		sb.WriteString("\nrequire (\n")
		sb.WriteString("\tgorm.io/gorm v1.25.12\n")
		sb.WriteString(")\n")
	}

	return sb.String()
}

// copyRuntimeFiles 复制 runtime 目录下的 .go 文件
func copyRuntimeFiles(runtimeDir, outDir string, verbose bool) error {
	// 检查 runtime 目录是否存在
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		return nil // runtime 目录不存在，跳过
	}

	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return &stdlibReadDirError{path: runtimeDir, err: err}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		srcFile := filepath.Join(runtimeDir, entry.Name())
		source, err := os.ReadFile(srcFile)
		if err != nil {
			return &stdlibReadFileError{path: srcFile, err: err}
		}

		outFile := filepath.Join(outDir, entry.Name())
		if err := os.WriteFile(outFile, source, 0644); err != nil {
			return &stdlibWriteError{path: outFile, err: err}
		}

		if verbose {
			printInfo(i18n.T(i18n.MsgCopyingStdlib, entry.Name(), outFile))
		}
	}

	// 为 runtime 包生成 go.mod 文件
	goModPath := filepath.Join(outDir, "go.mod")
	goModContent := "module tugo/runtime\n\ngo 1.21\n"
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return &stdlibWriteError{path: goModPath, err: err}
	}

	return nil
}

// 标准库相关错误类型
type stdlibReadDirError struct {
	path string
	err  error
}

func (e *stdlibReadDirError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrStdlibReadError), e.path, e.err)
}

type stdlibReadFileError struct {
	path string
	err  error
}

func (e *stdlibReadFileError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrStdlibReadError), e.path, e.err)
}

type stdlibParseError struct {
	path string
	msg  string
}

func (e *stdlibParseError) Error() string {
	return i18n.T(i18n.ErrStdlibParseError, e.path, e.msg)
}

type stdlibWriteError struct {
	path string
	err  error
}

func (e *stdlibWriteError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrStdlibWriteError), e.path, e.err)
}

type stdlibTranspileError struct {
	err error
}

func (e *stdlibTranspileError) Error() string {
	return fmt.Sprintf("%s: %v", i18n.T(i18n.ErrStdlibTranspile), e.err)
}

type createDirError struct {
	path string
	err  error
}

func (e *createDirError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrCannotCreateDir), e.path, e.err)
}
