// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"regexp"
	"sort"
	"strings"
)

// Highlight renders source code to colored HTML spans, one small regex
// tokenizer per language family. It is deliberately tiny: a tech journal
// wants readable coloring, not a compiler front end. Unknown languages
// render as plain escaped text.
func Highlight(lang, code string) string {
	rules := languageRules(strings.ToLower(lang))
	if rules == nil {
		return escapeHTML(code)
	}

	type match struct {
		start, end int
		class      string
	}
	var matches []match
	for _, rule := range rules {
		for _, loc := range rule.re.FindAllStringIndex(code, -1) {
			if loc[1] == loc[0] {
				continue
			}
			matches = append(matches, match{loc[0], loc[1], rule.class})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return matches[i].end > matches[j].end
	})

	var out strings.Builder
	last := 0
	for i := 0; i < len(matches); i++ {
		m := matches[i]
		if m.start < last {
			continue // overlapped by an earlier, longer match
		}
		out.WriteString(escapeHTML(code[last:m.start]))
		out.WriteString(`<span class="tok-` + m.class + `">`)
		out.WriteString(escapeHTML(code[m.start:m.end]))
		out.WriteString(`</span>`)
		last = m.end
	}
	out.WriteString(escapeHTML(code[last:]))
	return out.String()
}

type rule struct {
	re    *regexp.Regexp
	class string
}

func languageRules(lang string) []rule {
	bashKeywords := `\b(if|then|else|elif|fi|for|while|until|do|done|case|esac|function|in|select|time|coproc|return|break|continue|exit|local|export|readonly|declare|unset|shift|source|trap|set|eval|exec)\b`
	goKeywords := `\b(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|interface|map|package|range|return|select|struct|switch|type|var|nil|true|false|iota|make|new|len|cap|append|copy|delete|panic|recover)\b`
	cKeywords := `\b(break|case|const|continue|default|do|else|enum|extern|for|goto|if|inline|register|restrict|return|sizeof|static|struct|switch|typedef|union|volatile|while|NULL|true|false|include|define|ifndef|endif|pragma)\b`
	jsKeywords := `\b(break|case|catch|class|const|continue|default|delete|do|else|export|extends|finally|for|function|if|import|in|instanceof|let|new|return|static|super|switch|this|throw|try|typeof|var|void|while|yield|async|await|of|true|false|null|undefined)\b`
	pyKeywords := `\b(and|as|assert|async|await|break|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|nonlocal|not|or|pass|raise|return|try|while|with|yield|True|False|None|self)\b`
	rustKeywords := `\b(as|async|await|break|const|continue|crate|dyn|else|enum|extern|fn|for|if|impl|in|let|loop|match|mod|move|mut|pub|ref|return|self|static|struct|super|trait|type|unsafe|use|where|while|Some|None|Ok|Err)\b`

	stringRule := rule{regexp.MustCompile("(\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`[^`]*`|\"\"\"[\\s\\S]*?\"\"\")"), "str"}
	commentHash := rule{regexp.MustCompile(`#[^\n]*`), "com"}
	commentSlash := rule{regexp.MustCompile(`//[^\n]*|/\*[\s\S]*?\*/`), "com"}
	number := rule{regexp.MustCompile(`\b(0x[0-9a-fA-F]+|\d+\.?\d*(?:[eE][+-]?\d+)?)\b`), "num"}
	funcCall := rule{regexp.MustCompile(`\b([a-zA-Z_][\w]*)\s*\(`), "fn"}
	types := rule{regexp.MustCompile(`\b(?:string|int|int64|uint|uint64|float64|bool|byte|rune|error|any|interface|map|chan)\b`), "typ"}

	switch lang {
	case "bash", "sh", "shell", "zsh", "console", "terminal":
		return []rule{
			commentHash,
			stringRule,
			rule{regexp.MustCompile(`(?m)^[\w_]+=`), "var"},
			rule{regexp.MustCompile(`\$\{?[\w@#?]+\}?`), "var"},
			rule{regexp.MustCompile(bashKeywords), "kw"},
			funcCall,
			number,
		}
	case "go", "golang":
		return []rule{
			commentSlash,
			stringRule,
			rule{regexp.MustCompile(goKeywords), "kw"},
			types,
			funcCall,
			number,
		}
	case "c", "cpp", "c++", "h", "hpp", "objc":
		return []rule{
			commentSlash,
			stringRule,
			rule{regexp.MustCompile(`#\s*\w+[^\n]*`), "meta"},
			rule{regexp.MustCompile(cKeywords), "kw"},
			funcCall,
			number,
		}
	case "js", "javascript", "ts", "typescript", "jsx", "tsx", "mjs":
		return []rule{
			commentSlash,
			stringRule,
			rule{regexp.MustCompile(jsKeywords), "kw"},
			funcCall,
			number,
		}
	case "py", "python":
		return []rule{
			commentHash,
			stringRule,
			rule{regexp.MustCompile(`@\w+`), "meta"},
			rule{regexp.MustCompile(pyKeywords), "kw"},
			funcCall,
			number,
		}
	case "rs", "rust":
		return []rule{
			commentSlash,
			stringRule,
			rule{regexp.MustCompile(rustKeywords), "kw"},
			funcCall,
			number,
		}
	case "json":
		return []rule{
			rule{regexp.MustCompile(`"(?:[^"\\]|\\.)*"\s*:`), "key"},
			stringRule,
			number,
			rule{regexp.MustCompile(`\b(true|false|null)\b`), "kw"},
		}
	case "yaml", "yml", "toml", "ini":
		return []rule{
			commentHash,
			rule{regexp.MustCompile(`(?m)^[\w.-]+(?=\s*[:=])`), "key"},
			stringRule,
			number,
			rule{regexp.MustCompile(`\b(true|false|yes|no|on|off|null)\b`), "kw"},
		}
	case "css":
		return []rule{
			rule{regexp.MustCompile(`/\*[\s\S]*?\*/`), "com"},
			rule{regexp.MustCompile(`[.#]?[\w-]+(?=\s*\{)`), "sel"},
			rule{regexp.MustCompile(`[\w-]+(?=\s*:)`), "key"},
			stringRule,
			number,
		}
	case "html", "xml", "svg", "markup":
		return []rule{
			rule{regexp.MustCompile(`<!--[\s\S]*?-->`), "com"},
			rule{regexp.MustCompile(`</?[\w-]+`), "kw"},
			rule{regexp.MustCompile(`[\w-]+(?==)`), "key"},
			stringRule,
		}
	case "diff", "patch":
		return []rule{
			rule{regexp.MustCompile(`(?m)^\+[^\n]*`), "add"},
			rule{regexp.MustCompile(`(?m)^-[^\n]*`), "del"},
			rule{regexp.MustCompile(`(?m)^@@[^\n]*`), "meta"},
		}
	case "make", "makefile":
		return []rule{
			commentHash,
			rule{regexp.MustCompile(`(?m)^[\w.%-]+\s*(?::=|=|:)`), "key"},
			stringRule,
			number,
		}
	default:
		return nil
	}
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
