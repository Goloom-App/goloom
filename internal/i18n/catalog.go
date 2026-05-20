package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	localesbundle "git.f4mily.net/goloom/locales"
)

// DefaultLanguage is used when Accept-Language is missing or unsupported.
const DefaultLanguage = "en"

// SupportedLanguages lists locale files shipped with the binary.
var SupportedLanguages = []string{"en", "de"}

type localeFile struct {
	API map[string]string `json:"api"`
}

// Catalog resolves API error message keys per language.
type Catalog struct {
	messages map[string]map[string]string
}

// Load reads embedded locale JSON files.
func Load() (*Catalog, error) {
	messages := make(map[string]map[string]string)
	for _, lang := range SupportedLanguages {
		raw, err := fs.ReadFile(localesbundle.FS, lang+".json")
		if err != nil {
			return nil, fmt.Errorf("read locale %s: %w", lang, err)
		}
		var file localeFile
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("parse locale %s: %w", lang, err)
		}
		messages[lang] = file.API
	}
	return &Catalog{messages: messages}, nil
}

// LanguageFromRequest picks the best supported language from Accept-Language.
func LanguageFromRequest(r *http.Request) string {
	if r == nil {
		return DefaultLanguage
	}
	return MatchLanguage(r.Header.Get("Accept-Language"))
}

// MatchLanguage parses Accept-Language and returns a supported code or DefaultLanguage.
func MatchLanguage(header string) string {
	if strings.TrimSpace(header) == "" {
		return DefaultLanguage
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		if idx := strings.Index(tag, ";"); idx >= 0 {
			tag = strings.TrimSpace(tag[:idx])
		}
		tag = strings.ToLower(tag)
		for _, supported := range SupportedLanguages {
			if tag == supported || strings.HasPrefix(tag, supported+"-") {
				return supported
			}
		}
		primary := tag
		if dash := strings.Index(tag, "-"); dash > 0 {
			primary = tag[:dash]
		}
		for _, supported := range SupportedLanguages {
			if primary == supported {
				return supported
			}
		}
	}
	return DefaultLanguage
}

// Message returns the translated API string for key, falling back to English then key.
func (c *Catalog) Message(lang, key string) string {
	if c == nil {
		return key
	}
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = DefaultLanguage
	}
	if msg, ok := c.messages[lang][key]; ok && msg != "" {
		return msg
	}
	if lang != DefaultLanguage {
		if msg, ok := c.messages[DefaultLanguage][key]; ok && msg != "" {
			return msg
		}
	}
	return key
}

// WriteError writes a localized plain-text HTTP error body.
func (c *Catalog) WriteError(w http.ResponseWriter, r *http.Request, key string, status int) {
	http.Error(w, c.Message(LanguageFromRequest(r), key), status)
}
