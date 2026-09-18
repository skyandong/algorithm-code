// noteslint 守住笔记仓库的结构一致：索引数字、文件引用、README 覆盖、
// 实验编号、章节连续性、断言消息、禁用符号
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// violation 是一条待修的结构性不一致
type violation struct {
	file string
	line int
	msg  string
}

func (v violation) String() string {
	if v.line > 0 {
		return fmt.Sprintf("%s:%d: %s", v.file, v.line, v.msg)
	}
	return fmt.Sprintf("%s: %s", v.file, v.msg)
}

func main() {
	root := flag.String("root", ".", "仓库根目录")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var vs []violation
	vs = append(vs, checkIndexCounts(abs)...)
	vs = append(vs, checkReadmeCounts(abs)...)
	vs = append(vs, checkBrokenRefs(abs)...)
	vs = append(vs, checkReadmeCoverage(abs)...)
	vs = append(vs, checkBannedSymbols(abs)...)
	vs = append(vs, checkAssertMessages(abs)...)
	vs = append(vs, checkExperimentNumbering(abs)...)
	vs = append(vs, checkSectionNumbering(abs)...)

	if len(vs) == 0 {
		fmt.Println("notes 结构检查通过")
		return
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].file != vs[j].file {
			return vs[i].file < vs[j].file
		}
		return vs[i].line < vs[j].line
	})
	for _, v := range vs {
		fmt.Println(v)
	}
	fmt.Fprintf(os.Stderr, "\n共 %d 处不一致，见上方明细\n", len(vs))
	os.Exit(1)
}

// rel 把绝对路径折算成相对仓库根的展示路径
func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

// lineAt 返回字节偏移对应的行号
func lineAt(src string, off int) int {
	if off > len(src) {
		off = len(src)
	}
	return strings.Count(src[:off], "\n") + 1
}

// skipDir 判定不必检查的目录
func skipDir(name string) bool {
	switch name {
	case ".git", ".workbuddy", "node_modules", "vendor", "ent":
		return true
	}
	return false
}

// walkFiles 收集 dir 下指定后缀的文件，跳过无关目录
func walkFiles(dir, ext string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ext) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

var numberedNoteRe = regexp.MustCompile(`^\d{2}-.+\.md$`)

// isNumberedNote 判定文件名是否为编号分册
func isNumberedNote(name string) bool {
	return numberedNoteRe.MatchString(name)
}

// noteFiles 列出每个模块下的编号分册
func noteFiles(root string) []string {
	mods, err := os.ReadDir(filepath.Join(root, "notes"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range mods {
		if !e.IsDir() || skipDir(e.Name()) {
			continue
		}
		dir := filepath.Join(root, "notes", e.Name())
		for _, p := range walkFiles(dir, ".md") {
			if isNumberedNote(filepath.Base(p)) {
				out = append(out, p)
			}
		}
	}
	return out
}

// moduleDirs 列出 notes 下的一级模块目录
func moduleDirs(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "notes"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !skipDir(e.Name()) {
			out = append(out, filepath.Join(root, "notes", e.Name()))
		}
	}
	return out
}

var (
	declaredNotesRe = regexp.MustCompile(`笔记正文共 (\d+) 篇`)
	declaredMDRe    = regexp.MustCompile(`md 文件 (\d+) 个`)
)

// checkIndexCounts 核对 INDEX.md 声明的篇数与实际一致
func checkIndexCounts(root string) []violation {
	indexPath := filepath.Join(root, "INDEX.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return []violation{{file: "INDEX.md", msg: "读不到索引文件：" + err.Error()}}
	}
	text := string(data)
	notes := noteFiles(root)
	allMD := walkFiles(filepath.Join(root, "notes"), ".md")

	var vs []violation
	if m := declaredNotesRe.FindStringSubmatchIndex(text); m != nil {
		declared, _ := strconv.Atoi(text[m[2]:m[3]])
		if declared != len(notes) {
			vs = append(vs, violation{file: "INDEX.md", line: lineAt(text, m[0]),
				msg: fmt.Sprintf("声明正文 %d 篇，实际 %d 篇", declared, len(notes))})
		}
	} else {
		vs = append(vs, violation{file: "INDEX.md", msg: "缺少「笔记正文共 N 篇」的声明"})
	}
	if m := declaredMDRe.FindStringSubmatchIndex(text); m != nil {
		declared, _ := strconv.Atoi(text[m[2]:m[3]])
		if declared != len(allMD) {
			vs = append(vs, violation{file: "INDEX.md", line: lineAt(text, m[0]),
				msg: fmt.Sprintf("声明 notes 下 md %d 个，实际 %d 个", declared, len(allMD))})
		}
	}
	return vs
}

var (
	readmeNotesRe  = regexp.MustCompile(`(\d+) 篇笔记`)
	readmeModuleRe = regexp.MustCompile(`([a-z][a-z-]*)/\s+#\s*(\d+) 篇`)
)

// readmeCodeCounts 是 README 目录树里以「N 道」计数的刷题区
var readmeCodeCounts = []struct {
	dir string
	re  *regexp.Regexp
}{
	{"algorithms/leetcode", regexp.MustCompile(`leetcode/\s+#\s*(\d+) 道题`)},
	{"algorithms/leetcode-core", regexp.MustCompile(`leetcode-core/\s+#\s*(\d+) 道 S 级题`)},
}

// countGoFiles 数目录下的非测试 go 文件
func countGoFiles(dir string) int {
	n := 0
	for _, p := range walkFiles(dir, ".go") {
		if !strings.HasSuffix(p, "_test.go") {
			n++
		}
	}
	return n
}

// checkReadmeCounts 核对 README 声明的篇数与题数同实际一致
func checkReadmeCounts(root string) []violation {
	path := filepath.Join(root, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(data)

	perModule := map[string]int{}
	for _, p := range noteFiles(root) {
		perModule[filepath.Base(filepath.Dir(p))]++
	}

	var vs []violation
	if m := readmeNotesRe.FindStringSubmatchIndex(text); m != nil {
		declared, _ := strconv.Atoi(text[m[2]:m[3]])
		if total := len(noteFiles(root)); declared != total {
			vs = append(vs, violation{file: "README.md", line: lineAt(text, m[0]),
				msg: fmt.Sprintf("声明 %d 篇笔记，实际 %d 篇", declared, total)})
		}
	}
	for _, m := range readmeModuleRe.FindAllStringSubmatchIndex(text, -1) {
		mod := text[m[2]:m[3]]
		actual, known := perModule[mod]
		if !known {
			continue
		}
		if declared, _ := strconv.Atoi(text[m[4]:m[5]]); declared != actual {
			vs = append(vs, violation{file: "README.md", line: lineAt(text, m[0]),
				msg: fmt.Sprintf("声明 %s %d 篇，实际 %d 篇", mod, declared, actual)})
		}
	}
	for _, c := range readmeCodeCounts {
		m := c.re.FindStringSubmatchIndex(text)
		if m == nil {
			continue
		}
		if declared, _ := strconv.Atoi(text[m[2]:m[3]]); declared != countGoFiles(filepath.Join(root, c.dir)) {
			vs = append(vs, violation{file: "README.md", line: lineAt(text, m[0]),
				msg: fmt.Sprintf("声明 %s %d 道，实际 %d 道", c.dir, declared, countGoFiles(filepath.Join(root, c.dir)))})
		}
	}
	return vs
}

var (
	mdLinkRe  = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	codeRefRe = regexp.MustCompile("`((?:notes|demos|rpc|web|algorithms|engineering|scripts)/[A-Za-z0-9_./\\x{4e00}-\\x{9fa5}-]+\\.(?:go|md|proto|yml|yaml|conf|thrift|json|sh))`")
)

// mdLine 是一行 markdown 的内容与其起始字节偏移
type mdLine struct {
	off  int
	text string
}

// mdLines 按行切分并保留每行起始偏移，顺序稳定
func mdLines(text string) []mdLine {
	var out []mdLine
	off := 0
	for _, line := range strings.Split(text, "\n") {
		out = append(out, mdLine{off: off, text: line})
		off += len(line) + 1
	}
	return out
}

// markdownProse 把围栏代码块（必要时连同行内码）替换成等长空白，保留字节偏移
func markdownProse(text string, dropInline bool) string {
	out := []byte(text)
	inFence := false
	for _, ln := range mdLines(text) {
		trimmed := strings.TrimSpace(ln.text)
		switch {
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
			inFence = !inFence
			blankRange(out, ln.off, ln.off+len(ln.text))
		case inFence:
			blankRange(out, ln.off, ln.off+len(ln.text))
		case dropInline:
			blankInlineCode(out, ln.off, ln.text)
		}
	}
	return string(out)
}

// blankRange 把 [from, to) 的字节替换成空格，换行符保留
func blankRange(b []byte, from, to int) {
	for i := from; i < to && i < len(b); i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}

// blankInlineCode 抹掉一行里的 `...` 行内码
func blankInlineCode(b []byte, off int, line string) {
	for i := 0; i < len(line); i++ {
		if line[i] != '`' {
			continue
		}
		end := strings.IndexByte(line[i+1:], '`')
		if end < 0 {
			return
		}
		blankRange(b, off+i, off+i+1+end+1)
		i += end + 1
	}
}

// checkBrokenRefs 校验 md 里指向仓库内部的文件引用真实存在
func checkBrokenRefs(root string) []violation {
	var vs []violation
	for _, f := range walkFiles(root, ".md") {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		text := string(data)
		links := markdownProse(text, true)
		refs := markdownProse(text, false)

		for _, m := range mdLinkRe.FindAllStringSubmatchIndex(links, -1) {
			target := links[m[2]:m[3]]
			target = strings.Trim(target, "<>")
			if !internalRef(target) {
				continue
			}
			target = strings.SplitN(target, "#", 2)[0]
			if existsFrom(f, root, target) {
				continue
			}
			vs = append(vs, violation{file: rel(root, f), line: lineAt(text, m[0]),
				msg: "链接指向的文件不存在：" + target})
		}

		for _, m := range codeRefRe.FindAllStringSubmatchIndex(refs, -1) {
			target := refs[m[2]:m[3]]
			if existsFrom(f, root, target) {
				continue
			}
			vs = append(vs, violation{file: rel(root, f), line: lineAt(text, m[0]),
				msg: "正文引用的文件不存在：" + target})
		}
	}
	return vs
}

// internalRef 判定链接是否指向仓库内部
func internalRef(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") {
		return false
	}
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return false
	}
	return !strings.HasPrefix(target, "/")
}

// existsFrom 依次按「相对引用方目录」「相对仓库根」解析路径
func existsFrom(refFile, root, target string) bool {
	clean := filepath.Clean(strings.TrimPrefix(target, "./"))
	for _, base := range []string{filepath.Dir(refFile), root} {
		if _, err := os.Stat(filepath.Join(base, clean)); err == nil {
			return true
		}
	}
	return false
}

// checkReadmeCoverage 校验每个编号分册都被本模块 README 收录
func checkReadmeCoverage(root string) []violation {
	var vs []violation
	for _, mod := range moduleDirs(root) {
		readme := filepath.Join(mod, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			continue
		}
		linked := map[string]bool{}
		for _, m := range mdLinkRe.FindAllStringSubmatch(markdownProse(string(data), true), -1) {
			linked[filepath.Base(strings.SplitN(m[1], "#", 2)[0])] = true
		}
		entries, err := os.ReadDir(mod)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !isNumberedNote(e.Name()) || linked[e.Name()] {
				continue
			}
			vs = append(vs, violation{file: rel(root, readme), msg: "README 漏收分册：" + e.Name()})
		}
	}
	return vs
}

// checkBannedSymbols 校验全仓 md 不再出现禁用符号
func checkBannedSymbols(root string) []violation {
	var vs []violation
	for _, f := range walkFiles(root, ".md") {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "§") {
				vs = append(vs, violation{file: rel(root, f), line: i + 1,
					msg: "出现禁用符号 §，章节引用改写为「第 N 节」"})
			}
		}
	}
	return vs
}

// assertionCallRe 匹配 testify 的 assert / require 调用，捕获包名与方法名
var assertionCallRe = regexp.MustCompile(`\b(assert|require)\.(\w+)\(`)

// assertionHelpers 是包内不产生断言的构造与工具函数，不要求消息
var assertionHelpers = map[string]bool{"New": true}

// assertMinArgs 记录各断言的固定参数个数，不含末尾可选的消息；assert 与 require 签名一致
var assertMinArgs = map[string]int{
	"Equal": 3, "NotEqual": 3, "EqualValues": 3, "Exactly": 3, "ElementsMatch": 3,
	"True": 2, "False": 2, "Nil": 2, "NotNil": 2, "Empty": 2, "NotEmpty": 2,
	"Error": 2, "NoError": 2, "Zero": 2, "NotZero": 2, "Positive": 2, "Negative": 2,
	"Len": 3, "Contains": 3, "NotContains": 3, "Subset": 3, "NotSubset": 3,
	"ErrorIs": 3, "NotErrorIs": 3, "Regexp": 3, "NotRegexp": 3, "EqualError": 3,
	"Greater": 3, "GreaterOrEqual": 3, "Less": 3, "LessOrEqual": 3,
	"Panics": 2, "NotPanics": 2, "PanicsWithValue": 3, "InDelta": 4,
	"JSONEq": 3, "YAMLEq": 3, "Same": 3, "NotSame": 3, "IsType": 3, "Implements": 3,
	"WithinDuration": 4,
}

// checkAssertMessages 校验 experiments 里每条 assert / require 断言的末参是消息字符串
func checkAssertMessages(root string) []violation {
	var vs []violation
	for _, mod := range moduleDirs(root) {
		dir := filepath.Join(mod, "experiments")
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		for _, p := range walkFiles(dir, ".go") {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			src := string(data)
			for _, loc := range assertionCallRe.FindAllStringSubmatchIndex(src, -1) {
				open := loc[1] - 1
				end := matchParen(src, open)
				if end < 0 {
					continue
				}
				pkg := src[loc[2]:loc[3]]
				name := src[loc[4]:loc[5]]
				if assertionHelpers[name] {
					continue
				}
				fixed, known := assertMinArgs[name]
				if !known {
					vs = append(vs, violation{file: rel(root, p), line: lineAt(src, loc[0]),
						msg: "未登记的断言方法 " + pkg + "." + name + "，补进 assertMinArgs 后才判定消息"})
					continue
				}
				args := splitArgs(src[open : end+1])
				if len(args) < fixed {
					continue
				}
				if len(args) > fixed && isStringLit(strings.TrimSpace(args[fixed])) {
					continue
				}
				vs = append(vs, violation{file: rel(root, p), line: lineAt(src, loc[0]),
					msg: "断言缺消息：" + pkg + "." + name})
			}
		}
	}
	return vs
}

// matchParen 返回与下标 open 处左括号配对的右括号下标，未闭合返回 -1
func matchParen(src string, open int) int {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '"', '\'':
			i = skipQuoted(src, i, src[i])
		case '`':
			for i++; i < len(src) && src[i] != '`'; i++ {
			}
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// skipQuoted 返回从下标 i 处的引号开始、与之配对的收尾引号下标
func skipQuoted(src string, i int, quote byte) int {
	for i++; i < len(src) && src[i] != quote; i++ {
		if src[i] == '\\' {
			i++
		}
	}
	return i
}

// splitArgs 按顶层逗号切分括号内实参，括号与引号内的逗号不参与切分
func splitArgs(call string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(call, "("), ")")
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '"', '\'':
			i = skipQuoted(inner, i, inner[i])
		case '`':
			for i++; i < len(inner) && inner[i] != '`'; i++ {
			}
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, inner[start:i])
				start = i + 1
			}
		}
	}
	if strings.TrimSpace(inner[start:]) != "" {
		out = append(out, inner[start:])
	}
	return out
}

// isStringLit 判定实参是否为字符串字面量
func isStringLit(arg string) bool {
	return arg != "" && (arg[0] == '"' || arg[0] == '`')
}

// checkExperimentNumbering 校验 golang 实验文件名编号都有对应分册
func checkExperimentNumbering(root string) []violation {
	chapters := map[string]bool{}
	for _, p := range noteFiles(root) {
		if filepath.Base(filepath.Dir(p)) == "golang" {
			chapters[filepath.Base(p)[:2]] = true
		}
	}

	dir := filepath.Join(root, "notes", "golang", "experiments")
	var vs []violation
	for _, p := range walkFiles(dir, ".go") {
		name := filepath.Base(p)
		if len(name) < 2 {
			continue
		}
		if _, err := strconv.Atoi(name[:2]); err != nil {
			continue
		}
		if !chapters[name[:2]] {
			vs = append(vs, violation{file: rel(root, p),
				msg: "实验编号 " + name[:2] + " 没有对应的 notes/golang/" + name[:2] + "-*.md 分册"})
		}
	}
	return vs
}

var numberedHeadingRe = regexp.MustCompile(`^## (\d+)\.`)

// checkSectionNumbering 校验「## N.」编号在同一组内连续，允许分组后重新从 1 起
func checkSectionNumbering(root string) []violation {
	var vs []violation
	for _, p := range noteFiles(root) {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := string(data)
		prev := 0
		for _, ln := range mdLines(text) {
			m := numberedHeadingRe.FindStringSubmatch(ln.text)
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			if n != 1 && n != prev+1 {
				vs = append(vs, violation{file: rel(root, p), line: lineAt(text, ln.off),
					msg: fmt.Sprintf("章节编号不连续：上一节是 %d，本节写成 %d", prev, n)})
			}
			prev = n
		}
	}
	return vs
}
