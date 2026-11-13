// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"fmt"
	"html"
	"strings"
)

// MathHTML renders a small TeX subset to styled HTML — no JavaScript, no
// web fonts, no outbound calls. Supported: ^{} and _{} scripts, \frac{a}{b},
// \sqrt{x}, big operators (\sum \prod \int) with limits, \text{...},
// \mathrm{...}, \mathbf{...}, \mathcal-ish accents are not supported, symbol
// macros (\alpha … \omega, \times, \le, \ge, \ne, \approx, \to, \infty, …),
// {grouping}, and plain characters. Anything unknown renders its macro name
// verbatim so the source stays visible and the author can fix it.
func MathHTML(src string) string {
	var b strings.Builder
	p := &mathParser{src: src}
	p.renderGroup(&b, '\x00')
	return b.String()
}

type mathParser struct {
	src string
	pos int
}

func (p *mathParser) eof() bool { return p.pos >= len(p.src) }

func (p *mathParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

// renderGroup consumes until stop (or end), emitting HTML.
func (p *mathParser) renderGroup(b *strings.Builder, stop byte) {
	for !p.eof() {
		c := p.src[p.pos]
		if c == stop {
			p.pos++
			return
		}
		switch c {
		case '{':
			p.pos++
			p.renderGroup(b, '}')
		case '}':
			// stray close: treat as literal
			b.WriteString("}")
			p.pos++
		case '^', '_':
			tag := "sup"
			if c == '_' {
				tag = "sub"
			}
			p.pos++
			fmt.Fprintf(b, "<%s class=\"m-%s\">", tag, tag)
			p.renderScriptArg(b)
			fmt.Fprintf(b, "</%s>", tag)
		case '\\':
			p.renderMacro(b)
		case ' ', '\t', '\n':
			p.pos++
		default:
			b.WriteString(html.EscapeString(string(c)))
			p.pos++
		}
	}
}

// renderScriptArg renders the argument of ^ or _: one group or one token.
func (p *mathParser) renderScriptArg(b *strings.Builder) {
	if p.peek() == '{' {
		p.pos++
		p.renderGroup(b, '}')
		return
	}
	if p.peek() == '\\' {
		p.renderMacro(b)
		return
	}
	if !p.eof() {
		b.WriteString(html.EscapeString(string(p.src[p.pos])))
		p.pos++
	}
}

// renderMacro handles a \macro and its arguments.
func (p *mathParser) renderMacro(b *strings.Builder) {
	p.pos++ // consume backslash
	start := p.pos
	for !p.eof() && isMacroChar(p.src[p.pos]) {
		p.pos++
	}
	name := p.src[start:p.pos]

	if sym, ok := mathSymbols[name]; ok {
		b.WriteString(sym)
		return
	}
	switch name {
	case "frac", "dfrac", "tfrac":
		num := p.readGroup()
		den := p.readGroup()
		fmt.Fprintf(b, "<span class=\"m-frac\"><span class=\"m-frac-num\">%s</span><span class=\"m-frac-den\">%s</span></span>", MathHTML(num), MathHTML(den))
	case "sqrt":
		arg := p.readGroup()
		fmt.Fprintf(b, "<span class=\"m-sqrt\">&radic;<span class=\"m-sqrt-body\">%s</span></span>", MathHTML(arg))
	case "text", "mathrm", "operatorname":
		arg := p.readGroup()
		fmt.Fprintf(b, "<span class=\"m-text\">%s</span>", html.EscapeString(arg))
	case "mathbf", "bm", "boldsymbol":
		arg := p.readGroup()
		fmt.Fprintf(b, "<span class=\"m-bold\">%s</span>", MathHTML(arg))
	case "mathit", "italic":
		arg := p.readGroup()
		fmt.Fprintf(b, "<span class=\"m-it\">%s</span>", MathHTML(arg))
	case "sum", "prod", "coprod", "int", "iint", "oint", "bigcup", "bigcap", "bigoplus", "bigotimes":
		op := map[string]string{
			"sum": "&sum;", "prod": "&prod;", "coprod": "&coprod;", "int": "&int;",
			"iint": "&iint;", "oint": "&oint;", "bigcup": "&#8899;", "bigcap": "&#8898;",
			"bigoplus": "&#8853;", "bigotimes": "&#8855;",
		}[name]
		fmt.Fprintf(b, "<span class=\"m-bigop\">%s</span>", op)
	case "left", "right":
		// sizing no-ops: the delimiter follows as a normal token
	case "limits", "nolimits", "displaystyle", "quad", "qquad":
		if name == "quad" || name == "qquad" {
			b.WriteString("<span class=\"m-space\"></span>")
		}
	default:
		fmt.Fprintf(b, `<span class="m-unknown">\%s</span>`, html.EscapeString(name))
	}
}

// readGroup consumes one {…} argument (or a single token) and returns it raw.
func (p *mathParser) readGroup() string {
	p.skipSpace()
	if p.peek() == '{' {
		p.pos++
		start := p.pos
		depth := 1
		for !p.eof() && depth > 0 {
			switch p.src[p.pos] {
			case '{':
				depth++
			case '}':
				depth--
			}
			p.pos++
		}
		body := p.src[start : p.pos-1]
		return body
	}
	if p.peek() == '\\' {
		start := p.pos
		p.pos++
		for !p.eof() && isMacroChar(p.src[p.pos]) {
			p.pos++
		}
		return p.src[start:p.pos]
	}
	if p.eof() {
		return ""
	}
	ch := string(p.src[p.pos])
	p.pos++
	return ch
}

func (p *mathParser) skipSpace() {
	for !p.eof() && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n') {
		p.pos++
	}
}

func isMacroChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// mathSymbols maps TeX macros to their Unicode or entity spelling.
var mathSymbols = map[string]string{
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "epsilon": "ε", "varepsilon": "ε",
	"zeta": "ζ", "eta": "η", "theta": "θ", "vartheta": "ϑ", "iota": "ι", "kappa": "κ",
	"lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ", "pi": "π", "rho": "ρ", "sigma": "σ",
	"tau": "τ", "upsilon": "υ", "phi": "φ", "varphi": "φ", "chi": "χ", "psi": "ψ", "omega": "ω",
	"Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ", "Pi": "Π",
	"Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",
	"times": "×", "div": "÷", "cdot": "·", "pm": "±", "mp": "∓", "ast": "∗",
	"leq": "≤", "le": "≤", "geq": "≥", "ge": "≥", "neq": "≠", "ne": "≠", "equiv": "≡",
	"approx": "≈", "sim": "∼", "propto": "∝", "ll": "≪", "gg": "≫",
	"to": "→", "rightarrow": "→", "leftarrow": "←", "Rightarrow": "⇒", "Leftarrow": "⇐",
	"mid": "∣", "vert": "|", "lvert": "|", "rvert": "|", "parallel": "∥", "perp": "⊥",
	"leftrightarrow": "↔", "Leftrightarrow": "⇔", "mapsto": "↦", "uparrow": "↑", "downarrow": "↓",
	"in": "∈", "notin": "∉", "ni": "∋", "subset": "⊂", "subseteq": "⊆", "supset": "⊃",
	"supseteq": "⊇", "cup": "∪", "cap": "∩", "setminus": "∖", "emptyset": "∅", "varnothing": "∅",
	"forall": "∀", "exists": "∃", "nexists": "∄", "neg": "¬", "land": "∧", "lor": "∨",
	"infty": "∞", "partial": "∂", "nabla": "∇", "aleph": "ℵ", "hbar": "ℏ", "ell": "ℓ",
	"langle": "⟨", "rangle": "⟩", "lceil": "⌈", "rceil": "⌉", "lfloor": "⌊", "rfloor": "⌋",
	"ldots": "…", "cdots": "⋯", "vdots": "⋮", "ddots": "⋱",
	"prime": "′", "circ": "∘", "bullet": "∙", "star": "⋆", "dagger": "†", "ddagger": "‡",
	"deg": "°", "circledast": "⊛", "oplus": "⊕", "ominus": "⊖", "otimes": "⊗",
	"max": "max", "min": "min", "log": "log", "ln": "ln", "exp": "exp", "sin": "sin",
	"cos": "cos", "tan": "tan", "sinh": "sinh", "cosh": "cosh", "tanh": "tanh",
	"Re": "ℜ", "Im": "ℑ", "wp": "℘", "imath": "ı", "jmath": "ȷ",
}
