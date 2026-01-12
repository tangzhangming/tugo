package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tangzhangming/tugo/internal/i18n"
)

// runCmd 转译并运行 tugo 源码
func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	verbose := fs.Bool("v", false, i18n.T(i18n.MsgRunOptVerbose))

	fs.Usage = func() {
		fmt.Println(i18n.T(i18n.MsgRunUsage))
		fmt.Println()
		fmt.Println(i18n.T(i18n.MsgRunDescription))
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println(i18n.T(i18n.MsgRunArgInput))
		fmt.Println()
		fmt.Println("Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		printError(i18n.T(i18n.ErrInputRequired))
		fs.Usage()
		os.Exit(1)
	}

	input := fs.Arg(0)

	// 获取当前工作目录
	cwd, err := os.Getwd()
	if err != nil {
		printError(i18n.T(i18n.ErrCannotGetCwd, err))
		os.Exit(1)
	}

	// 输出目录为 .output
	outputDir := filepath.Join(cwd, ".output")

	// 清理并创建输出目录
	if err := os.RemoveAll(outputDir); err != nil {
		printError(i18n.T(i18n.ErrCannotCleanDir, err))
		os.Exit(1)
	}

	// 转译
	if err := transpileInput(input, outputDir, *verbose); err != nil {
		printError("Error: " + err.Error())
		os.Exit(1)
	}

	// 运行
	if *verbose {
		printInfo(i18n.T(i18n.MsgRunning))
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
			printError(i18n.T(i18n.ErrRunError, err))
			os.Exit(1)
		}
	} else {
		// 目录模式，运行整个包
		cmd := exec.Command("go", "run", ".")
		cmd.Dir = runDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			printError(i18n.T(i18n.ErrRunError, err))
			os.Exit(1)
		}
	}
}
