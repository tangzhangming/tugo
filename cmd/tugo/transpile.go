package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/i18n"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
	"github.com/tangzhangming/tugo/internal/transpiler"
)

// transpileInput 转译输入文件或目录
func transpileInput(input, output string, verbose bool) error {
	info, err := os.Stat(input)
	if err != nil {
		return &accessError{err: err}
	}

	// 查找并加载 tugo.toml 配置
	startDir := input
	if !info.IsDir() {
		startDir = filepath.Dir(input)
	}

	cfg, configPath, err := config.FindAndLoad(startDir)
	if err != nil {
		return &configError{err: err}
	}

	if verbose {
		if configPath != "" {
			printInfo(i18n.T(i18n.MsgUsingConfig, configPath, cfg.Project.Module))
		} else {
			printInfo(i18n.T(i18n.MsgNoConfig, cfg.Project.Module))
		}
	}

	if info.IsDir() {
		return transpileDir(input, output, verbose, cfg)
	}
	return transpileFile(input, output, verbose, cfg)
}

// transpileDir 转译目录
func transpileDir(inputDir, outputDir string, verbose bool, cfg *config.Config) error {
	// 第一遍：收集所有文件并解析，构建全局符号表
	var allFiles []*parser.File
	fileMap := make(map[string]string) // 文件路径 -> 相对路径

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".tugo") {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		if verbose {
			printInfo(i18n.T(i18n.MsgParsing, path))
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return &readFileError{path: path, err: err}
		}

		file, errors := parser.Parse(string(source))
		if len(errors) > 0 {
			return &parseError{path: path, msg: errors[0]}
		}

		allFiles = append(allFiles, file)
		fileMap[path] = relPath

		return nil
	})

	if err != nil {
		return err
	}

	if len(allFiles) == 0 {
		return &noFilesError{dir: inputDir}
	}

	// 收集 tugo 标准库导入
	tugoImports := collectTugoImports(allFiles)

	// 预解析标准库文件，将类信息加载到全局符号表
	var stdlibFiles []*parser.File
	if len(tugoImports) > 0 {
		stdlibDir, err := getStdlibDir()
		if err == nil {
			stdlibFiles = preloadStdlibClasses(stdlibDir, tugoImports, verbose)
			if verbose {
				printInfo(fmt.Sprintf("预加载了 %d 个标准库文件", len(stdlibFiles)))
			}
		} else if verbose {
			printInfo(fmt.Sprintf("标准库目录不存在: %v", err))
		}
	}

	// 构建全局符号表（包含用户代码和标准库）
	allFilesWithStdlib := append(stdlibFiles, allFiles...)
	table := symbol.Collect(allFilesWithStdlib)

	// 第二遍：转译
	t := transpiler.New(table)
	t.SetConfig(cfg)

	i := 0
	err = filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".tugo") {
			return nil
		}

		relPath := fileMap[path]
		outputPath := filepath.Join(outputDir, strings.TrimSuffix(relPath, ".tugo")+".go")

		if verbose {
			printInfo(i18n.T(i18n.MsgTranspiling, path, outputPath))
		}

		// 确保输出目录存在
		outputDirPath := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDirPath, 0755); err != nil {
			return &createDirError{path: outputDirPath, err: err}
		}

		// 获取文件名（不含路径和后缀）用于入口类检测
		baseName := filepath.Base(path)
		fileName := strings.TrimSuffix(baseName, ".tugo")

		// 转译（传递文件名用于入口类检测）
		goCode, err := t.TranspileFileWithName(allFiles[i], fileName)
		if err != nil {
			return &transpileError{path: path, err: err}
		}
		i++

		// 写入输出文件
		if err := os.WriteFile(outputPath, []byte(goCode), 0644); err != nil {
			return &writeFileError{path: outputPath, err: err}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 转译标准库到 vendor 目录
	if len(tugoImports) > 0 {
		stdlibDir, err := getStdlibDir()
		if err != nil {
			if verbose {
				printWarning(err.Error())
			}
		} else {
			if err := transpileStdlib(stdlibDir, outputDir, tugoImports, verbose, cfg); err != nil {
				return err
			}
		}
	}

	// 生成 go.mod 文件
	goModPath := filepath.Join(outputDir, "go.mod")
	goModContent := generateGoMod(cfg.Project.Module, tugoImports)
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return &goModError{err: err}
	}

	if verbose {
		printInfo(i18n.T(i18n.MsgTranspileSuccess, len(allFiles)))
		printInfo(i18n.T(i18n.MsgGeneratedGoMod, cfg.Project.Module))
	}

	return nil
}

// transpileFile 转译单个文件
func transpileFile(inputFile, outputPath string, verbose bool, cfg *config.Config) error {
	if verbose {
		printInfo(i18n.T(i18n.MsgParsing, inputFile))
	}

	source, err := os.ReadFile(inputFile)
	if err != nil {
		return &readFileError{path: inputFile, err: err}
	}

	// 解析
	file, errors := parser.Parse(string(source))
	if len(errors) > 0 {
		return &parseError{path: inputFile, msg: errors[0]}
	}

	// 收集 tugo 标准库导入
	tugoImports := collectTugoImports([]*parser.File{file})

	// 预解析标准库文件，将类信息加载到全局符号表
	var stdlibFiles []*parser.File
	if len(tugoImports) > 0 {
		stdlibDir, err := getStdlibDir()
		if err == nil {
			stdlibFiles = preloadStdlibClasses(stdlibDir, tugoImports, verbose)
		}
	}

	// 构建符号表（包含用户代码和标准库）
	allFilesWithStdlib := append(stdlibFiles, file)
	table := symbol.Collect(allFilesWithStdlib)

	// 获取文件名（不含路径和后缀）用于入口类检测
	baseName := filepath.Base(inputFile)
	fileName := strings.TrimSuffix(baseName, ".tugo")

	// 转译（传递文件名用于入口类检测）
	t := transpiler.New(table)
	t.SetConfig(cfg)
	goCode, err := t.TranspileFileWithName(file, fileName)
	if err != nil {
		return &transpileError{path: inputFile, err: err}
	}

	// 确定输出路径
	finalOutput := outputPath
	info, err := os.Stat(outputPath)
	if err == nil && info.IsDir() {
		// 输出是目录，使用输入文件名
		baseName := filepath.Base(inputFile)
		baseName = strings.TrimSuffix(baseName, ".tugo") + ".go"
		finalOutput = filepath.Join(outputPath, baseName)
	} else if !strings.HasSuffix(outputPath, ".go") {
		// 确保输出目录存在，然后在其中创建文件
		if err := os.MkdirAll(outputPath, 0755); err != nil {
			return &createDirError{path: outputPath, err: err}
		}
		baseName := filepath.Base(inputFile)
		baseName = strings.TrimSuffix(baseName, ".tugo") + ".go"
		finalOutput = filepath.Join(outputPath, baseName)
	}

	// 确保输出目录存在
	outputDir := filepath.Dir(finalOutput)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return &createDirError{path: outputDir, err: err}
	}

	if verbose {
		printInfo(i18n.T(i18n.MsgTranspiling, inputFile, finalOutput))
	}

	// 写入输出文件
	if err := os.WriteFile(finalOutput, []byte(goCode), 0644); err != nil {
		return &writeFileError{path: finalOutput, err: err}
	}

	// 转译标准库到 vendor 目录
	if len(tugoImports) > 0 {
		stdlibDir, err := getStdlibDir()
		if err != nil {
			if verbose {
				printWarning(err.Error())
			}
		} else {
			if err := transpileStdlib(stdlibDir, outputDir, tugoImports, verbose, cfg); err != nil {
				return err
			}
		}
	}

	// 生成 go.mod 文件
	goModPath := filepath.Join(outputDir, "go.mod")
	goModContent := generateGoMod(cfg.Project.Module, tugoImports)
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return &goModError{err: err}
	}

	if verbose {
		printInfo(i18n.T(i18n.MsgTranspileSuccess, 1))
		printInfo(i18n.T(i18n.MsgGeneratedGoMod, cfg.Project.Module))
	}

	return nil
}

// 错误类型定义
type accessError struct {
	err error
}

func (e *accessError) Error() string {
	return fmt.Sprintf("%s: %v", i18n.T(i18n.ErrCannotAccessInput), e.err)
}

type configError struct {
	err error
}

func (e *configError) Error() string {
	return fmt.Sprintf("%s: %v", i18n.T(i18n.ErrCannotLoadConfig), e.err)
}

type readFileError struct {
	path string
	err  error
}

func (e *readFileError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrCannotReadFile), e.path, e.err)
}

type parseError struct {
	path string
	msg  string
}

func (e *parseError) Error() string {
	return i18n.T(i18n.ErrParseError, e.path, e.msg)
}

type transpileError struct {
	path string
	err  error
}

func (e *transpileError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrTranspileError, ""), e.path, e.err)
}

type noFilesError struct {
	dir string
}

func (e *noFilesError) Error() string {
	return i18n.T(i18n.ErrNoTugoFiles, e.dir)
}

type writeFileError struct {
	path string
	err  error
}

func (e *writeFileError) Error() string {
	return fmt.Sprintf("%s %s: %v", i18n.T(i18n.ErrCannotWriteFile), e.path, e.err)
}

type goModError struct {
	err error
}

func (e *goModError) Error() string {
	return fmt.Sprintf("%s: %v", i18n.T(i18n.ErrCannotWriteGoMod), e.err)
}

// generateGoMod 生成 go.mod 文件内容
// 如果使用了 tugo.db，会自动添加 GORM 依赖
func generateGoMod(module string, tugoImports map[string]bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("module %s\n\ngo 1.21\n", module))

	// 检查是否使用了 tugo.db
	if tugoImports["tugo.db"] {
		sb.WriteString("\nrequire (\n")
		sb.WriteString("\tgorm.io/gorm v1.25.12\n")
		sb.WriteString("\tgorm.io/driver/mysql v1.5.7\n")
		sb.WriteString("\tgorm.io/driver/postgres v1.5.11\n")
		sb.WriteString("\tgorm.io/driver/sqlite v1.5.7\n")
		sb.WriteString(")\n")
	}

	// 添加 replace 指令，将 tugo 包映射到本地目录
	if len(tugoImports) > 0 {
		sb.WriteString("\nreplace (\n")
		for pkgPath := range tugoImports {
			// tugo.db -> tugo/db
			goPkgPath := strings.ReplaceAll(pkgPath, ".", "/")
			sb.WriteString(fmt.Sprintf("\t%s => ./%s\n", goPkgPath, goPkgPath))
		}
		sb.WriteString(")\n")
	}

	return sb.String()
}
