// Package i18n provides internationalization support for tugo compiler.
package i18n

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
)

// Language represents a supported language
type Language string

const (
	LangEnglish Language = "en"
	LangChinese Language = "zh"
)

var (
	currentLang Language
	once        sync.Once
)

// Init initializes the i18n system by detecting the system language.
// This is called automatically on first use, but can be called explicitly.
func Init() {
	once.Do(func() {
		currentLang = detectLanguage()
	})
}

// SetLanguage sets the current language manually.
func SetLanguage(lang Language) {
	currentLang = lang
}

// GetLanguage returns the current language.
func GetLanguage() Language {
	Init()
	return currentLang
}

// T translates a message key to the current language.
// If the key is not found, returns the key itself.
// Supports format arguments like fmt.Sprintf.
func T(key string, args ...any) string {
	Init()
	
	var messages map[string]string
	switch currentLang {
	case LangChinese:
		messages = zhMessages
	default:
		messages = enMessages
	}
	
	template, ok := messages[key]
	if !ok {
		// Fallback to English
		template, ok = enMessages[key]
		if !ok {
			// Return key if not found
			return key
		}
	}
	
	if len(args) > 0 {
		return fmt.Sprintf(template, args...)
	}
	return template
}

// detectLanguage detects the system language.
func detectLanguage() Language {
	// Check environment variables first (works on all platforms)
	for _, envVar := range []string{"TUGO_LANG", "LANG", "LC_ALL", "LANGUAGE"} {
		if lang := os.Getenv(envVar); lang != "" {
			if detected := parseLanguageCode(lang); detected != "" {
				return detected
			}
		}
	}
	
	// Platform-specific detection
	if runtime.GOOS == "windows" {
		return detectWindowsLanguage()
	}
	
	// Default to English
	return LangEnglish
}

// parseLanguageCode parses a language code string and returns the Language.
func parseLanguageCode(code string) Language {
	code = strings.ToLower(code)
	
	// Handle formats like "zh_CN.UTF-8", "zh-CN", "zh", "en_US", etc.
	if strings.HasPrefix(code, "zh") {
		return LangChinese
	}
	if strings.HasPrefix(code, "en") {
		return LangEnglish
	}
	
	return ""
}

// detectWindowsLanguage detects language on Windows.
func detectWindowsLanguage() Language {
	// Try to get the user's preferred UI language from environment
	// Windows sets these in some configurations
	if lang := os.Getenv("LANG"); lang != "" {
		if detected := parseLanguageCode(lang); detected != "" {
			return detected
		}
	}
	
	// Default to English on Windows
	// Note: For full Windows language detection, we would need to use
	// syscall to call GetUserDefaultUILanguage, but that adds complexity.
	// Environment variables and TUGO_LANG should be sufficient for most users.
	return LangEnglish
}
