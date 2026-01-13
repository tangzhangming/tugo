package i18n

// Message keys for parser errors
const (
	// Parser errors
	ErrExpectedToken = "parser.expected_token" // args: line, column, expected, got
	ErrGeneric       = "parser.generic"        // args: line, column, message
)

// Message keys for transpiler errors
const (
	// Interface implementation errors
	ErrInterfaceNotFound        = "transpiler.interface_not_found"          // args: className, interfaceName
	ErrMissingMethod            = "transpiler.missing_method"               // args: className, interfaceName, methodName
	ErrParamCountMismatch       = "transpiler.param_count_mismatch"         // args: className, methodName, got, interfaceName, expected
	ErrReturnCountMismatch      = "transpiler.return_count_mismatch"        // args: className, methodName, got, interfaceName, expected
	ErrStructInterfaceNotFound  = "transpiler.struct_interface_not_found"   // args: structName, interfaceName
	ErrStructMissingMethod      = "transpiler.struct_missing_method"        // args: structName, interfaceName, methodName
	ErrStructParamMismatch      = "transpiler.struct_param_mismatch"        // args: structName, methodName, got, interfaceName, expected
	ErrStructReturnMismatch     = "transpiler.struct_return_mismatch"       // args: structName, methodName, got, interfaceName, expected

	// Extends/inheritance errors
	ErrParentClassNotFound    = "transpiler.parent_class_not_found"     // args: className, parentName
	ErrExtendNonAbstract      = "transpiler.extend_non_abstract"        // args: className, parentName
	ErrAbstractMethodMissing  = "transpiler.abstract_method_missing"    // args: className, methodName, parentName
	ErrAbstractParamMismatch  = "transpiler.abstract_param_mismatch"    // args: className, methodName, got, parentName, expected
	ErrAbstractReturnMismatch = "transpiler.abstract_return_mismatch"   // args: className, methodName, got, parentName, expected

	// Static class errors
	ErrStaticClassInit     = "transpiler.static_class_init"      // args: className
	ErrStaticClassThis     = "transpiler.static_class_this"      // args: className, methodName

	// Top-level statement errors
	ErrTopLevelFunction = "transpiler.top_level_function" // args: funcName
	ErrTopLevelVariable = "transpiler.top_level_variable" // args: varName
	ErrTopLevelConstant = "transpiler.top_level_constant" // args: constName

	// File naming errors
	ErrTooManyPublicTypes  = "transpiler.too_many_public_types"  // args: fileName, count
	ErrPublicClassFileName = "transpiler.public_class_filename"  // args: className, expectedFile, actualFile
	ErrPublicIfaceFileName = "transpiler.public_iface_filename"  // args: interfaceName, expectedFile, actualFile

	// Main method errors
	ErrMainNotStatic   = "transpiler.main_not_static"     // args: className
	ErrMainNotPublic   = "transpiler.main_not_public"     // args: className
	ErrMainHasParams   = "transpiler.main_has_params"     // args: className
	ErrMainHasReturns  = "transpiler.main_has_returns"    // args: className

	// Errable function errors
	ErrErrableNotHandled       = "transpiler.errable_not_handled"        // args: funcName, calledFunc
	ErrErrableMethodNotHandled = "transpiler.errable_method_not_handled" // args: funcName, methodName

	// Symbol errors
	ErrUndefinedType   = "transpiler.undefined_type"    // args: typeName
	ErrUnusedImport    = "transpiler.unused_import"     // args: typeName, path

	// Codegen errors
	ErrTooManyVariables           = "codegen.too_many_variables"            // args: returnCount, assignCount
	ErrErrableMultiReturnNoAssign = "codegen.errable_multi_return_no_assign" // args: returnCount

	// Overload errors
	ErrDuplicateOverloadSignature = "transpiler.duplicate_overload_signature" // args: className, methodName, signature
	ErrOverloadOnlyReturnDiffers  = "transpiler.overload_only_return_differs" // args: className, methodName

	// Visibility errors
	ErrPrivateMethodAccess = "transpiler.private_method_access" // args: callerClass, targetClass, methodName
	ErrPrivateFieldAccess  = "transpiler.private_field_access"  // args: callerClass, targetClass, fieldName

	// Ternary expression errors
	ErrTernaryTypeMismatch = "codegen.ternary_type_mismatch" // args: trueType, falseType
)

// Message keys for CLI
const (
	// Usage and help
	MsgUsage            = "cli.usage"
	MsgCommands         = "cli.commands"
	MsgCmdRun           = "cli.cmd_run"
	MsgCmdBuild         = "cli.cmd_build"
	MsgCmdVersion       = "cli.cmd_version"
	MsgCmdHelp          = "cli.cmd_help"
	MsgUseHelp          = "cli.use_help"
	MsgUnknownCommand   = "cli.unknown_command"          // args: command

	// Run command
	MsgRunUsage         = "cli.run_usage"
	MsgRunDescription   = "cli.run_description"
	MsgRunArgInput      = "cli.run_arg_input"
	MsgRunOptVerbose    = "cli.run_opt_verbose"

	// Build command
	MsgBuildUsage       = "cli.build_usage"
	MsgBuildDescription = "cli.build_description"
	MsgBuildArgInput    = "cli.build_arg_input"
	MsgBuildOptOutput   = "cli.build_opt_output"
	MsgBuildOptVerbose  = "cli.build_opt_verbose"
	MsgBuildCompleted   = "cli.build_completed"          // args: outputDir
	MsgBuildCompletedV  = "cli.build_completed_verbose"  // args: outputDir

	// Common errors
	ErrInputRequired    = "cli.input_required"
	ErrCannotGetCwd     = "cli.cannot_get_cwd"           // args: error
	ErrCannotCleanDir   = "cli.cannot_clean_dir"         // args: error
	ErrCannotAccessInput = "cli.cannot_access_input"     // args: error
	ErrCannotLoadConfig = "cli.cannot_load_config"       // args: error
	ErrCannotReadFile   = "cli.cannot_read_file"         // args: path, error
	ErrParseError       = "cli.parse_error"              // args: path, error
	ErrTranspileError   = "cli.transpile_error"          // args: path, error
	ErrNoTugoFiles      = "cli.no_tugo_files"            // args: dir
	ErrCannotCreateDir  = "cli.cannot_create_dir"        // args: path, error
	ErrCannotWriteFile  = "cli.cannot_write_file"        // args: path, error
	ErrCannotWriteGoMod = "cli.cannot_write_gomod"       // args: error
	ErrRunError         = "cli.run_error"                // args: error

	// Info messages
	MsgUsingConfig      = "cli.using_config"             // args: configPath, module
	MsgNoConfig         = "cli.no_config"                // args: module
	MsgParsing          = "cli.parsing"                  // args: path
	MsgTranspiling      = "cli.transpiling"              // args: input, output
	MsgRunning          = "cli.running"
	MsgTranspileSuccess = "cli.transpile_success"        // args: count
	MsgGeneratedGoMod   = "cli.generated_gomod"          // args: module

	// Stdlib messages
	MsgStdlibNotFound    = "cli.stdlib_not_found"         // args: path
	MsgStdlibPkgNotFound = "cli.stdlib_pkg_not_found"     // args: pkgPath
	MsgParsingStdlib     = "cli.parsing_stdlib"           // args: path
	MsgTranspilingStdlib = "cli.transpiling_stdlib"       // args: input, output
	MsgCopyingStdlib     = "cli.copying_stdlib"           // args: input, output
	ErrStdlibReadError   = "cli.stdlib_read_error"        // args: path, error
	ErrStdlibParseError  = "cli.stdlib_parse_error"       // args: path, error
	ErrStdlibWriteError  = "cli.stdlib_write_error"       // args: path, error
	ErrStdlibTranspile   = "cli.stdlib_transpile_error"   // args: error
)
