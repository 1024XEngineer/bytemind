package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Terminal interface {
	Printf(format string, args ...any)
	Println(args ...any)
	PromptYesNo(question string) (bool, error)
}

type Console struct {
	reader        *bufio.Reader
	out           io.Writer
	colorsEnabled bool
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
)

func NewConsole(in io.Reader, out io.Writer) *Console {
	return &Console{reader: bufio.NewReader(in), out: out, colorsEnabled: true}
}

func (c *Console) Printf(format string, args ...any) {
	fmt.Fprintf(c.out, format, args...)
}

func (c *Console) Println(args ...any) {
	fmt.Fprintln(c.out, args...)
}

func (c *Console) PromptYesNo(question string) (bool, error) {
	line, err := c.PromptLine(c.Style(question+" [y/N]: ", ansiYellow))
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func (c *Console) PromptLine(prompt string) (string, error) {
	if _, err := fmt.Fprint(c.out, prompt); err != nil {
		return "", err
	}
	line, err := c.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (c *Console) PrintBox(title string, lines ...string) {
	width := len(title) + 4
	for _, line := range lines {
		if len(line)+4 > width {
			width = len(line) + 4
		}
	}
	border := "+" + strings.Repeat("-", width-2) + "+"
	middle := "+" + strings.Repeat("-", width-2) + "+"
	bottom := "+" + strings.Repeat("-", width-2) + "+"
	fmt.Fprintln(c.out, c.Style(border, ansiBlue))
	fmt.Fprintf(c.out, "%s\n", c.Style(fmt.Sprintf("| %-*s |", width-4, title), ansiBlue+ansiBold))
	if len(lines) > 0 {
		fmt.Fprintln(c.out, c.Style(middle, ansiBlue))
		for _, line := range lines {
			fmt.Fprintf(c.out, "%s\n", fmt.Sprintf("| %-*s |", width-4, line))
		}
	}
	fmt.Fprintln(c.out, c.Style(bottom, ansiBlue))
}

func (c *Console) Banner(title string) {
	fmt.Fprintln(c.out, c.Style(title, ansiBold+ansiBlue))
}

func (c *Console) Info(text string) {
	fmt.Fprintln(c.out, c.Style(text, ansiDim))
}

func (c *Console) Success(text string) {
	fmt.Fprintln(c.out, c.Style(text, ansiGreen))
}

func (c *Console) Warn(text string) {
	fmt.Fprintln(c.out, c.Style(text, ansiYellow))
}

func (c *Console) Error(text string) {
	fmt.Fprintln(c.out, c.Style(text, ansiRed))
}

func (c *Console) Assistant(text string) {
	c.printRole("assistant", ansiGreen, text)
}

func (c *Console) System(text string) {
	c.printRole("system", ansiBlue, text)
}

func (c *Console) Tool(text string) {
	c.printRole("tool", ansiCyan, text)
}

func (c *Console) Style(text, style string) string {
	if !c.colorsEnabled {
		return text
	}
	return style + text + ansiReset
}

func (c *Console) printRole(role, color, text string) {
	label := c.Style(role+":", ansiBold+color)
	parts := strings.Split(strings.TrimSpace(text), "\n")
	if len(parts) == 0 {
		fmt.Fprintln(c.out, label)
		return
	}
	fmt.Fprintln(c.out, label, parts[0])
	for _, line := range parts[1:] {
		fmt.Fprintln(c.out, " ", line)
	}
}
