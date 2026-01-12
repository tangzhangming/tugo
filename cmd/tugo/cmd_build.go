package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tangzhangming/tugo/internal/i18n"
)

// buildCmd 转译 tugo 源码到 Go
func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	outputDir := fs.String("o", "output", i18n.T(i18n.MsgBuildOptOutput))
	verbose := fs.Bool("v", false, i18n.T(i18n.MsgBuildOptVerbose))

	fs.Usage = func() {
		fmt.Println(i18n.T(i18n.MsgBuildUsage))
		fmt.Println()
		fmt.Println(i18n.T(i18n.MsgBuildDescription))
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println(i18n.T(i18n.MsgBuildArgInput))
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

	if err := transpileInput(input, *outputDir, *verbose); err != nil {
		printError("Error: " + err.Error())
		os.Exit(1)
	}

	if *verbose {
		fmt.Println(i18n.T(i18n.MsgBuildCompletedV, *outputDir))
	} else {
		fmt.Println(i18n.T(i18n.MsgBuildCompleted, *outputDir))
	}
}
