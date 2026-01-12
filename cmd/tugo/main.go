package main

import (
	"fmt"
	"os"

	"github.com/tangzhangming/tugo/internal/i18n"
)

const version = "0.1.0"

func main() {
	// 初始化国际化
	i18n.Init()

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
		printError(i18n.T(i18n.MsgUnknownCommand, os.Args[1]))
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(i18n.T(i18n.MsgUsage))
	fmt.Println()
	fmt.Println(i18n.T(i18n.MsgCommands))
	fmt.Println(i18n.T(i18n.MsgCmdRun))
	fmt.Println(i18n.T(i18n.MsgCmdBuild))
	fmt.Println(i18n.T(i18n.MsgCmdVersion))
	fmt.Println(i18n.T(i18n.MsgCmdHelp))
	fmt.Println()
	fmt.Println(i18n.T(i18n.MsgUseHelp))
}

// 辅助打印函数
func printError(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

func printInfo(msg string) {
	fmt.Println(msg)
}

func printWarning(msg string) {
	fmt.Println("Warning:", msg)
}
