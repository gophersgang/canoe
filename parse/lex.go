package parse

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type item struct {
	typ itemType
	pos Pos
	val string
}

func (i item) String() string {
	switch {
	case i.typ == itemEOF:
		return "EOF"
	case i.typ == itemError:
		return i.val
	case i.typ > itemKeyword:
		return fmt.Sprintf("<%s>", i.val)
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

type itemType int

const (
	itemError itemType = iota
	itemEOF

	itemText
	itemLeftDelim
	itemRightDelim
	itemColonEqual
	itemNumber
	itemChar
	itemString
	itemVariable
	itemSpace

	itemKeyword
	itemIf
	itemElse
	itemRange
	itemFunc
	itemNil
	itemComplex
	itemIdentifier
	itemImport
)

const eof = -1

const (
	leftDelim = "<="
	rightDelim = "=>"

	leftComment = "/*"
	rightComment = "*/"
)

var keywords = map[string]itemType{
	"if": itemIf,
	"else": itemElse,
	"range": itemRange,
	"nil": itemNil,
	"func": itemFunc,
	"import": itemImport,
}

type stateFn func(*lexer) stateFn

type lexer struct {
	name    string
	input   string
	state   stateFn
	pos     Pos
	start   Pos
	width   Pos
	lastPos Pos
	items   chan item
}

func lex(name, input string) *lexer {
	l := &lexer{
		name: name,
		input: input,
		items: make(chan item),
	}

	go l.run()
	return l
}

// return the next rune from input
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.width = 0
		return eof
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = Pos(w)
	l.pos += l.width
	return r
}

func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *lexer) backup() {
	l.pos -= l.width
}

func (l *lexer) emit(typ itemType) {
	l.items <- item{typ, l.start, l.input[l.start:l.pos]}
	l.start = l.pos
}

func (l *lexer) ignore() {
	l.start = l.pos
}

func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}

	l.backup()
	return false
}

func (l *lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	l.backup()
}

func (l *lexer) lineNumber() int {
	return 1 + strings.Count(l.input[:l.lastPos], "\n")
}

func (l *lexer) errorf(format string, args ...interface{}) stateFn {
	l.items <- item{itemError, l.start, fmt.Sprintf(format, args)}
	return nil
}

func (l *lexer) nextItem() item {
	item := <-l.items
	l.lastPos = item.pos
	return item
}

func (l *lexer) drain() {
	for range l.items {
	}
}

func (l *lexer) run() {
	l.state = lexText
	for l.state != nil {
		l.state = l.state(l)	
	}

	close(l.items)
}

func lexText(l *lexer) stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], leftDelim) {
			if l.pos > l.start {
				l.emit(itemText)
			}	
			return lexLeftDelim
		}

		if l.next() == eof {
			break
		}
	}

	if l.pos > l.start {
		l.emit(itemText)
	}
	l.emit(itemEOF)
	return nil
}

func lexLeftDelim(l *lexer) stateFn {
	l.pos += Pos(len(leftDelim))
	if strings.HasPrefix(l.input[l.pos:], leftComment) {
		return lexComment	
	}
	l.emit(itemLeftDelim)

	return lexInsideBlock
}

func lexComment(l *lexer) stateFn {
	l.pos += Pos(len(leftComment))
	i := strings.Index(l.input[l.pos:], rightComment)
	if i < 0 {
		return l.errorf("unclosed comment")
	}
	l.pos += Pos(i + len(rightComment))
	if !strings.HasPrefix(l.input[l.pos:], rightDelim) {
		return l.errorf("comment ends before closing delimiter")
	}
	l.pos += Pos(len(rightDelim))
	l.ignore()
	return lexText
}

func lexRightDelim(l *lexer) stateFn {
	l.pos += Pos(len(rightDelim))
	l.emit(itemRightDelim)
	return lexText
}

func lexInsideBlock(l *lexer) stateFn {
	
	if strings.HasPrefix(l.input[l.pos:], rightDelim) {
		return lexRightDelim
	}
	switch r := l.next(); {
	case isSpace(r):
		return lexSpace
	case r == '"':
		return lexQuote
	case isAlphaNumeric(r):
		l.backup()
		return lexIdentifier
	}

	return lexInsideBlock
}

func lexSpace(l *lexer) stateFn {
	for isSpace(l.peek()) {
		l.next()
	}
	l.emit(itemSpace)
	return lexInsideBlock
}

func lexIdentifier(l *lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):

		default:
			l.backup()
			word := l.input[l.start:l.pos]
			switch {
			case keywords[word] > itemKeyword:
				l.emit(keywords[word])
			default:
				l.emit(itemIdentifier)
			}
			break Loop
		}	
	}

	return lexInsideBlock
}

func lexQuote(l *lexer) stateFn {
Loop:
	for {
		switch l.next() {
		case '\\':
			if r := l.next(); r != eof && r != '\n' {
				break
			}
			fallthrough
		case eof, '\n':
			return l.errorf("unterminated string")
		case '"':
			break Loop
		}
	}
	l.emit(itemString)
	return lexInsideBlock
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
