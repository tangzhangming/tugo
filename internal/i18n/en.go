package i18n

// enMessages contains English translations
var enMessages = map[string]string{
	// Parser errors
	ErrExpectedToken: "line %d:%d: expected %s, got %s",
	ErrGeneric:       "line %d:%d: %s",

	// Interface implementation errors
	ErrInterfaceNotFound:       "class %s: interface %s not found",
	ErrMissingMethod:           "class %s does not implement interface %s: missing method %s",
	ErrParamCountMismatch:      "class %s method %s: parameter count mismatch (got %d, interface %s requires %d)",
	ErrReturnCountMismatch:     "class %s method %s: return value count mismatch (got %d, interface %s requires %d)",
	ErrStructInterfaceNotFound: "struct %s: interface %s not found",
	ErrStructMissingMethod:     "struct %s does not implement interface %s: missing method %s",
	ErrStructParamMismatch:     "struct %s method %s: parameter count mismatch (got %d, interface %s requires %d)",
	ErrStructReturnMismatch:    "struct %s method %s: return value count mismatch (got %d, interface %s requires %d)",

	// Extends/inheritance errors
	ErrParentClassNotFound:    "class %s: parent class %s not found",
	ErrExtendNonAbstract:      "class %s: cannot extend non-abstract class %s",
	ErrAbstractMethodMissing:  "class %s does not implement abstract method %s from parent class %s",
	ErrAbstractParamMismatch:  "class %s method %s: parameter count mismatch (got %d, abstract method in %s requires %d)",
	ErrAbstractReturnMismatch: "class %s method %s: return value count mismatch (got %d, abstract method in %s requires %d)",

	// Static class errors
	ErrStaticClassInit: "static class %s cannot have init constructor",
	ErrStaticClassThis: "static class %s method %s cannot use 'this', use 'self::' instead",

	// Top-level statement errors
	ErrTopLevelFunction: "functions must be defined inside a class, found top-level function '%s'",
	ErrTopLevelVariable: "variables must be defined inside a class, found top-level variable '%s'",
	ErrTopLevelConstant: "constants must be defined inside a class, found top-level constant '%s'",

	// File naming errors
	ErrTooManyPublicTypes:  "file '%s' has %d public classes/interfaces, only one is allowed",
	ErrPublicClassFileName: "public class '%s' must be in file '%s.tugo', but found in '%s.tugo'",
	ErrPublicIfaceFileName: "public interface '%s' must be in file '%s.tugo', but found in '%s.tugo'",

	// Main method errors
	ErrMainNotStatic:  "main method in class '%s' must be static",
	ErrMainNotPublic:  "main method in class '%s' must be public",
	ErrMainHasParams:  "main method in class '%s' cannot have parameters",
	ErrMainHasReturns: "main method in class '%s' cannot have return values",

	// Errable function errors
	ErrErrableNotHandled:       "function %s: call to errable function '%s' must be inside a try block or current function must be errable",
	ErrErrableMethodNotHandled: "function %s: call to errable method '%s' must be inside a try block or current function must be errable",

	// Symbol errors
	ErrUndefinedType: "undefined type '%s': not imported or defined",
	ErrUnusedImport:  "imported type '%s' is not used (from %s)",

	// Codegen errors
	ErrTooManyVariables:           "too many variables: function returns %d values but trying to assign to %d variables",
	ErrErrableMultiReturnNoAssign: "errable function with %d return values cannot be used as expression statement, must assign to variables",

	// Overload errors
	ErrDuplicateOverloadSignature: "class %s method %s: duplicate overload signature '%s'",
	ErrOverloadOnlyReturnDiffers:  "class %s method %s: overloaded methods cannot differ only by return type",

	// Visibility errors
	ErrPrivateMethodAccess: "%s: cannot access %s's private method '%s'",
	ErrPrivateFieldAccess:  "%s: cannot access %s's private field '%s'",

	// CLI - Usage and help
	MsgUsage:          "Usage: tugo <command> [arguments]",
	MsgCommands:       "Commands:",
	MsgCmdRun:         "  run      Transpile and run tugo source files",
	MsgCmdBuild:       "  build    Transpile tugo source files to Go",
	MsgCmdVersion:     "  version  Print version information",
	MsgCmdHelp:        "  help     Print this help message",
	MsgUseHelp:        "Use \"tugo <command> -h\" for more information about a command.",
	MsgUnknownCommand: "Unknown command: %s",

	// CLI - Run command
	MsgRunUsage:       "Usage: tugo run [options] <input>",
	MsgRunDescription: "Transpile tugo source files to Go and run them.\nOutput is placed in .output directory (auto-cleaned).",
	MsgRunArgInput:    "  <input>    Input file or directory",
	MsgRunOptVerbose:  "Verbose output",

	// CLI - Build command
	MsgBuildUsage:       "Usage: tugo build [options] <input>",
	MsgBuildDescription: "Transpile tugo source files to Go.",
	MsgBuildArgInput:    "  <input>    Input file or directory",
	MsgBuildOptOutput:   "Output directory",
	MsgBuildOptVerbose:  "Verbose output",
	MsgBuildCompleted:   "Build completed: %s",
	MsgBuildCompletedV:  "Build completed. Output: %s",

	// CLI - Common errors
	ErrInputRequired:     "Error: input file or directory is required",
	ErrCannotGetCwd:      "Error: cannot get current directory: %v",
	ErrCannotCleanDir:    "Error: cannot clean output directory: %v",
	ErrCannotAccessInput: "cannot access input",
	ErrCannotLoadConfig:  "cannot load config",
	ErrCannotReadFile:    "cannot read file",
	ErrParseError:        "parse error in %s: %s",
	ErrTranspileError:    "transpile error in %s",
	ErrNoTugoFiles:       "no .tugo files found in %s",
	ErrCannotCreateDir:   "cannot create output directory",
	ErrCannotWriteFile:   "cannot write file",
	ErrCannotWriteGoMod:  "cannot write go.mod",
	ErrRunError:          "Error running: %v",

	// CLI - Info messages
	MsgUsingConfig:      "Using config: %s (module: %s)",
	MsgNoConfig:         "No tugo.toml found, using default module: %s",
	MsgParsing:          "Parsing: %s",
	MsgTranspiling:      "Transpiling: %s -> %s",
	MsgRunning:          "Running...",
	MsgTranspileSuccess: "Successfully transpiled %d files",
	MsgGeneratedGoMod:   "Generated go.mod with module: %s",

	// CLI - Stdlib messages
	MsgStdlibNotFound:    "stdlib directory not found: %s",
	MsgStdlibPkgNotFound: "Warning: stdlib package not found: %s",
	MsgParsingStdlib:     "Parsing stdlib: %s",
	MsgTranspilingStdlib: "Transpiling stdlib: %s -> %s",
	MsgCopyingStdlib:     "Copying stdlib: %s -> %s",
	ErrStdlibReadError:   "cannot read stdlib file",
	ErrStdlibParseError:  "parse error in stdlib %s: %s",
	ErrStdlibWriteError:  "cannot write stdlib file",
	ErrStdlibTranspile:   "cannot transpile stdlib",
}
