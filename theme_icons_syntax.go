package rodlog

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/styles"
)

type DiagnosticIcons struct {
	Error       string `toml:"error"`
	Warning     string `toml:"warning"`
	Info        string `toml:"info"`
	Suggestion  string `toml:"suggestion"`
	Explanation string `toml:"explanation"`
}

type SyntaxTheme struct {
	Style   string `toml:"style"`
	Profile string `toml:"profile"`
	Lexer   string `toml:"lexer"`
}

type HighlightColors struct {
	Code     string `toml:"code"`
	Location string `toml:"location"`
	URL      string `toml:"url"`
	Error    string `toml:"error"`
	Warning  string `toml:"warning"`
	Success  string `toml:"success"`
	Duration string `toml:"duration"`
	CLI      string `toml:"cli"`
}

type ThemeBundle struct {
	Name        string
	Description string
	Syntax      SyntaxTheme
	Semantic    SemanticColors
	Docker      DockerColors
}

var builtinThemes = map[string]ThemeBundle{
	"default": {
		Name:        "default",
		Description: "Balanced ANSI-compatible semantic palette with Monokai syntax",
		Syntax:      SyntaxTheme{Style: "monokai", Profile: "truecolor"},
		Semantic:    SemanticColors{},
		Docker:      DefaultDockerColors(),
	},
	"tokyo-night": {
		Name:        "tokyo-night",
		Description: "Cool slate & neon pastel palette inspired by Tokyo Night",
		Syntax:      SyntaxTheme{Style: "tokyonight-night", Profile: "truecolor"},
		Semantic: SemanticColors{
			Error:    "#F7768E",
			Warning:  "#E0AF68",
			Success:  "#9ECE6A",
			Info:     "#7AA2F7",
			Muted:    "#565F89",
			Code:     "#BB9AF7",
			Location: "#7DCFFF",
		},
		Docker: DockerColors{
			BadgeBackground: "#1A1B26",
			BadgeText:       "#C0CAF5",
			Done:            "#9ECE6A",
			Cached:          "#565F89",
			Error:           "#F7768E",
			Separator:       "#7AA2F7",
		},
	},
	"dracula": {
		Name:        "dracula",
		Description: "High-contrast purple, cyan, and pink Dracula palette",
		Syntax:      SyntaxTheme{Style: "dracula", Profile: "truecolor"},
		Semantic: SemanticColors{
			Error:    "#FF5555",
			Warning:  "#F1FA8C",
			Success:  "#50FA7B",
			Info:     "#8BE9FD",
			Muted:    "#6272A4",
			Code:     "#BD93F9",
			Location: "#8BE9FD",
		},
		Docker: DockerColors{
			BadgeBackground: "#282A36",
			BadgeText:       "#F8F8F2",
			Done:            "#50FA7B",
			Cached:          "#6272A4",
			Error:           "#FF5555",
			Separator:       "#BD93F9",
		},
	},
	"catppuccin": {
		Name:        "catppuccin",
		Description: "Soothing pastel Catppuccin Mocha palette",
		Syntax:      SyntaxTheme{Style: "catppuccin-mocha", Profile: "truecolor"},
		Semantic: SemanticColors{
			Error:    "#F38BA8",
			Warning:  "#F9E2AF",
			Success:  "#A6E3A1",
			Info:     "#89B4FA",
			Muted:    "#6C7086",
			Code:     "#CBA6F7",
			Location: "#94E2D5",
		},
		Docker: DockerColors{
			BadgeBackground: "#1E1E2E",
			BadgeText:       "#CDD6F4",
			Done:            "#A6E3A1",
			Cached:          "#6C7086",
			Error:           "#F38BA8",
			Separator:       "#CBA6F7",
		},
	},
	"github-dark": {
		Name:        "github-dark",
		Description: "GitHub Actions & Primer dark mode palette",
		Syntax:      SyntaxTheme{Style: "github-dark", Profile: "truecolor"},
		Semantic: SemanticColors{
			Error:    "#FF7B72",
			Warning:  "#D29922",
			Success:  "#3FB950",
			Info:     "#58A6FF",
			Muted:    "#8B949E",
			Code:     "#D2A8FF",
			Location: "#79C0FF",
		},
		Docker: DockerColors{
			BadgeBackground: "#161B22",
			BadgeText:       "#F0F6FC",
			Done:            "#3FB950",
			Cached:          "#8B949E",
			Error:           "#FF7B72",
			Separator:       "#30363D",
		},
	},
	"nord": {
		Name:        "nord",
		Description: "Arctic, north-bluish low-glare palette",
		Syntax:      SyntaxTheme{Style: "nord", Profile: "truecolor"},
		Semantic: SemanticColors{
			Error:    "#BF616A",
			Warning:  "#EBCB8B",
			Success:  "#A3BE8C",
			Info:     "#81A1C1",
			Muted:    "#4C566A",
			Code:     "#B48EAD",
			Location: "#88C0D0",
		},
		Docker: DockerColors{
			BadgeBackground: "#2E3440",
			BadgeText:       "#ECEFF4",
			Done:            "#A3BE8C",
			Cached:          "#4C566A",
			Error:           "#BF616A",
			Separator:       "#81A1C1",
		},
	},
}

func BuiltinThemes() []ThemeBundle {
	out := make([]ThemeBundle, 0, len(builtinThemes))
	for _, bundle := range builtinThemes {
		out = append(out, bundle)
	}
	slices.SortFunc(out, func(a, b ThemeBundle) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func ResolveTheme(name string) (ThemeBundle, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = "default"
	}
	bundle, ok := builtinThemes[key]
	if !ok {
		return ThemeBundle{}, fmt.Errorf("unknown theme %q (available: catppuccin, default, dracula, github-dark, nord, tokyo-night)", name)
	}
	return bundle, nil
}

func iconOrDefault(icon, fallback string) string {
	if icon == "" {
		return fallback
	}
	return icon
}

func ParseDiagnosticIcons(input DiagnosticIcons) (DiagnosticIcons, error) {
	for name, icon := range map[string]string{
		"error": input.Error, "warning": input.Warning, "info": input.Info,
		"suggestion": input.Suggestion, "explanation": input.Explanation,
	} {
		if !utf8.ValidString(icon) || utf8.RuneCountInString(icon) > 12 {
			return DiagnosticIcons{}, fmt.Errorf("icons.%s: expected at most 12 valid Unicode characters", name)
		}
		for _, r := range icon {
			if r < 32 || r == 127 || r >= 0x80 && r <= 0x9f || r == 0x2028 || r == 0x2029 {
				return DiagnosticIcons{}, fmt.Errorf("icons.%s: control characters are not allowed", name)
			}
		}
	}
	return input, nil
}

func ParseSyntaxTheme(input SyntaxTheme) (SyntaxTheme, error) {
	if input.Style != "" {
		if strings.TrimSpace(input.Style) != input.Style || styles.Get(input.Style) == nil {
			return SyntaxTheme{}, fmt.Errorf("syntax.style: unknown Chroma style %q", input.Style)
		}
	}
	switch strings.ToLower(strings.TrimSpace(input.Profile)) {
	case "", "auto", "truecolor", "ansi256", "ansi16":
	default:
		return SyntaxTheme{}, fmt.Errorf("syntax.profile: invalid profile %q (expected auto|truecolor|ansi256|ansi16)", input.Profile)
	}
	return input, nil
}

func ParseHighlightColors(input HighlightColors) (HighlightColors, error) {
	fields := []struct {
		name  string
		value *string
	}{
		{"code", &input.Code},
		{"location", &input.Location},
		{"url", &input.URL},
		{"error", &input.Error},
		{"warning", &input.Warning},
		{"success", &input.Success},
		{"duration", &input.Duration},
		{"cli", &input.CLI},
	}
	for _, f := range fields {
		if *f.value == "" {
			continue
		}
		normalized, err := ColorSGR(*f.value, false)
		if err != nil {
			return HighlightColors{}, fmt.Errorf("highlight.%s: %w", f.name, err)
		}
		*f.value = normalized
	}
	return input, nil
}
