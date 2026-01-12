package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/tugo/internal/config"
	"github.com/tangzhangming/tugo/internal/parser"
	"github.com/tangzhangming/tugo/internal/symbol"
	"github.com/tangzhangming/tugo/internal/transpiler"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "build":
		buildCmd(os.Args[2:])
	case "version":
		fmt.Println("tugo version", version)
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: tugo <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  run      Transpile and run tugo source files")
	fmt.Println("  build    Transpile tugo source files to Go")
	fmt.Println("  version  Print version information")
	fmt.Println("  help     Print this help message")
	fmt.Println()
	fmt.Println("Use \"tugo <command> -h\" for more information about a command.")
}

// runCmd 转译并运行 tugo 源码
func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	verbose := fs.Bool("v", false, "Verbose output")

	fs.Usage = func() {
		fmt.Println("Usage: tugo run [options] <input>")
		fmt.Println()
		fmt.Println("Transpile tugo source files to Go and run them.")
		fmt.Println("Output is placed in .output directory (auto-cleaned).")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("  <input>    Input file or directory")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Println("Error: input file or directory is required")
		fs.Usage()
		os.Exit(1)
	}

	input := fs.Arg(0)

	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: cannot get current directory: %v\n", err)
		os.Exit(1)
	}

	// 输出目录为 .output
	outputDir := filepath.Join(cwd, ".output")

	// 清理并创建输出目录
	if err := os.RemoveAll(outputDir); err != nil {
		fmt.Printf("Error: cannot clean output directory: %v\n", err)
		os.Exit(1)
	}

	// 转译
	if err := transpileInput(input, outputDir, *verbose); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// 运行
	if *verbose {
		fmt.Println("Running...")
	}

	// 确定运行目录
	runDir := outputDir
	inputInfo, _ := os.Stat(input)
	if inputInfo != nil && !inputInfo.IsDir() {
		// 单文件模式，直接运行生成的 .go 文件
		baseName := filepath.Base(input)
		goFile := strings.TrimSuffix(baseName, ".tugo") + ".go"
		runDir = outputDir

		cmd := exec.Command("go", "run", goFile)
		cmd.Dir = runDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error running: %v\n", err)
			os.Exit(1)
		}
	} else {
		// 目录模式，运行整个包
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = runDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error running: %v\n", err)
			os.Exit(1)
		}
	}
}

// buildCmd 转译 tugo 源码到 Go
func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	outputDir := fs.String("o", "output", "Output directory")
	verbose := fs.Bool("v", false, "Verbose output")

	fs.Usage = func() {
		fmt.Println("Usage: tugo build [options] <input>")
		fmt.Println()
		fmt.Println("Transpile tugo source files to Go.")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("  <input>    Input file or directory")
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		fmt.Println("Error: input file or directory is required")
		fs.Usage()
		os.Exit(1)
	}

	input := fs.Arg(0)

	if err := transpileInput(input, *outputDir, *verbose); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Build completed. Output: %s\n", *outputDir)
	} else {
		fmt.Printf("Build completed: %s\n", *outputDir)
	}
}

func transpileInput(input, output string, verbose bool) error {
	info, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("cannot access input: %w", err)
	}

	// 查找并加载 tugo.toml 配置
	startDir := input
	if !info.IsDir() {
		startDir = filepath.Dir(input)
	}

	cfg, configPath, err := config.FindAndLoad(startDir)
	if err != nil {
		return fmt.Errorf("cannot load config: %w", err)
	}

	if verbose {
		if configPath != "" {
			fmt.Printf("Using config: %s (module: %s)\n", configPath, cfg.Project.Module)
		} else {
			fmt.Printf("No tugo.toml found, using default module: %s\n", cfg.Project.Module)
		}
	}

	if info.IsDir() {
		return transpileDir(input, output, verbose, cfg)
	}
	return transpileFile(input, output, verbose, cfg)
}

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
			fmt.Printf("Parsing: %s\n", path)
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read file %s: %w", path, err)
		}

		file, errors := parser.Parse(string(source))
		if len(errors) > 0 {
			return fmt.Errorf("parse error in %s: %s", path, errors[0])
		}

		allFiles = append(allFiles, file)
		fileMap[path] = relPath

		return nil
	})

	if err != nil {
		return err
	}

	if len(allFiles) == 0 {
		return fmt.Errorf("no .tugo files found in %s", inputDir)
	}

	// 构建全局符号表
	table := symbol.Collect(allFiles)

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
			fmt.Printf("Transpiling: %s -> %s\n", path, outputPath)
		}

		// 确保输出目录存在
		outputDirPath := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputDirPath, 0755); err != nil {
			return fmt.Errorf("cannot create output directory %s: %w", outputDirPath, err)
		}

		// 获取文件名（不含路径和后缀）用于入口类检测
		baseName := filepath.Base(path)
		fileName := strings.TrimSuffix(baseName, ".tugo")

		// 转译（传递文件名用于入口类检测）
		goCode, err := t.TranspileFileWithName(allFiles[i], fileName)
		if err != nil {
			return fmt.Errorf("transpile error in %s: %w", path, err)
		}
		i++

		// 写入输出文件
		if err := os.WriteFile(outputPath, []byte(goCode), 0644); err != nil {
			return fmt.Errorf("cannot write file %s: %w", outputPath, err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 生成 go.mod 文件
	goModPath := filepath.Join(outputDir, "go.mod")
	goModContent := fmt.Sprintf("module %s\n\ngo 1.21\n", cfg.Project.Module)
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return fmt.Errorf("cannot write go.mod: %w", err)
	}

	if verbose {
		fmt.Printf("Successfully transpiled %d files\n", len(allFiles))
		fmt.Printf("Generated go.mod with module: %s\n", cfg.Project.Module)
	}

	return nil
}

func transpileFile(inputFile, outputPath string, verbose bool, cfg *config.Config) error {
	if verbose {
		fmt.Printf("Parsing: %s\n", inputFile)
	}

	source, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}

	// 解析
	file, errors := parser.Parse(string(source))
	if len(errors) > 0 {
		return fmt.Errorf("parse error: %s", errors[0])
	}

	// 构建符号表
	table := symbol.Collect([]*parser.File{file})

	// 获取文件名（不含路径和后缀）用于入口类检测
	baseName := filepath.Base(inputFile)
	fileName := strings.TrimSuffix(baseName, ".tugo")

	// 转译（传递文件名用于入口类检测）
	t := transpiler.New(table)
	t.SetConfig(cfg)
	goCode, err := t.TranspileFileWithName(file, fileName)
	if err != nil {
		return fmt.Errorf("transpile error: %w", err)
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
			return fmt.Errorf("cannot create output directory: %w", err)
		}
		baseName := filepath.Base(inputFile)
		baseName = strings.TrimSuffix(baseName, ".tugo") + ".go"
		finalOutput = filepath.Join(outputPath, baseName)
	}

	// 确保输出目录存在
	outputDir := filepath.Dir(finalOutput)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	if verbose {
		fmt.Printf("Transpiling: %s -> %s\n", inputFile, finalOutput)
	}

	// 写入输出文件
	if err := os.WriteFile(finalOutput, []byte(goCode), 0644); err != nil {
		return fmt.Errorf("cannot write output file: %w", err)
	}

	if verbose {
		fmt.Println("Successfully transpiled")
	}

	return nil
}
