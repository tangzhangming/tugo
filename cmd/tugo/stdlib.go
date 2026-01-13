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
	stdlibRelDir    = "src"    // 标准库相对 tugo.exe 的目录
	stdlibPkgPrefix = "tugo."  // 标准库包前缀
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
					// tugo.lang.Str -> tugo/lang
					imports[spec.PkgPath] = true
				}
			}
		}
		
		// 检测是否有非静态类声明（需要隐式导入 tugo.lang 用于 ClassInfo）
		for _, stmt := range file.Statements {
			if classDecl, ok := stmt.(*parser.ClassDecl); ok {
				if !classDecl.Static {
					imports["tugo.lang"] = true
					break // 只需要检测到一个就够了
				}
			}
		}
	}
	return imports
}

// transpileStdlib 转译标准库到 vendor 目录
func transpileStdlib(stdlibDir, outputDir string, tugoImports map[string]bool, verbose bool, cfg *config.Config) error {
	if len(tugoImports) == 0 {
		return nil
	}

	vendorDir := filepath.Join(outputDir, "vendor")

	for pkgPath := range tugoImports {
		// tugo.lang -> lang/ (去掉标准库前缀，因为标准库都在 src/ 下)
		srcRelPath := pkgPath
		if strings.HasPrefix(srcRelPath, stdlibPkgPrefix) {
			srcRelPath = srcRelPath[len(stdlibPkgPrefix):] // 去掉前缀
		}
		srcRelPath = strings.ReplaceAll(srcRelPath, ".", string(filepath.Separator))
		srcDir := filepath.Join(stdlibDir, srcRelPath)

		// 检查源目录是否存在
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			if verbose {
				printInfo(i18n.T(i18n.MsgStdlibPkgNotFound, pkgPath))
			}
			continue
		}

		// 输出目录: vendor/tugo/lang/
		outPkgDir := filepath.Join(vendorDir, strings.ReplaceAll(pkgPath, ".", "/"))
		if err := os.MkdirAll(outPkgDir, 0755); err != nil {
			return &createDirError{path: outPkgDir, err: err}
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

			srcFile := filepath.Join(srcDir, entry.Name())
			source, err := os.ReadFile(srcFile)
			if err != nil {
				return &stdlibReadFileError{path: srcFile, err: err}
			}

			file, errors := parser.Parse(string(source))
			if len(errors) > 0 {
				return &stdlibParseError{path: srcFile, msg: errors[0]}
			}

			pkgFiles = append(pkgFiles, file)
			fileNames = append(fileNames, strings.TrimSuffix(entry.Name(), ".tugo"))

			if verbose {
				printInfo(i18n.T(i18n.MsgParsingStdlib, srcFile))
			}
		}

		// 复制所有 .go 文件（预编译的 Go 代码，如 class_info.go）
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
