/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package wtl reads what the node needs of the sources of pages, blocks and menus in the Weaver
// Template Language (weaver-next packages/wtl/docs/wtl-spec.md): the elements they refer to by
// name. WTL requires those names to be literals (STR-1, MenuItem page), so they are known without
// evaluating the source.
package wtl

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// Refs are the elements a source refers to: the blocks of its Include elements and the pages of
// its MenuItem elements. A name of the source's own ecosystem is kept without its prefix
// (@5name in ecosystem 5 is name); a name of another ecosystem keeps it.
type Refs struct {
	Blocks []string `json:"blocks,omitempty"`
	Pages  []string `json:"pages,omitempty"`
}

// JSON is the stored form of the refs: each list sorted, without repeats, and left out when
// empty; {} when the source refers to nothing.
func (r Refs) JSON() string {
	out, _ := json.Marshal(r)
	return string(out)
}

// IsSource tells whether a stored value is a WTL source: its first line is the @wtl header
// (SYN-1). The other values are templates of the legacy language, which refer to nothing here.
func IsSource(value string) bool {
	value = strings.TrimPrefix(value, "\uFEFF")
	return strings.HasPrefix(value, "@wtl") && (len(value) == 4 || value[4] == ' ' || value[4] == '\t')
}

// Read returns the refs of a source of an element of an ecosystem. It never fails: a source
// that is not WTL refers to nothing, and the text after a syntax error is not read, as the
// source does not render anyway.
func Read(value string, ecosystem int64) Refs {
	if !IsSource(value) {
		return Refs{}
	}
	s := &scanner{src: strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")}
	if i := strings.IndexByte(s.src, '\n'); i >= 0 {
		s.pos = i + 1
	} else {
		s.pos = len(s.src)
	}
	own := "@" + strconv.FormatInt(ecosystem, 10)
	var refs Refs
	for s.pos < len(s.src) {
		name, attrs, ok := s.next()
		if !ok {
			break
		}
		var target *[]string
		var attr string
		switch name {
		case "Include":
			target, attr = &refs.Blocks, "name"
		case "MenuItem":
			target, attr = &refs.Pages, "page"
		default:
			continue
		}
		if v, ok := attrs[attr]; ok && v != "" {
			if rest, found := strings.CutPrefix(v, own); found && rest != "" && (rest[0] < '0' || rest[0] > '9') {
				v = rest
			}
			*target = append(*target, v)
		}
	}
	refs.Blocks = sortedSet(refs.Blocks)
	refs.Pages = sortedSet(refs.Pages)
	return refs
}

func sortedSet(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	slices.Sort(list)
	return slices.Compact(list)
}

type scanner struct {
	src string
	pos int
}

// next reads up to the next opening tag and returns its name and its attributes whose values
// are string literals. ok is false at the end of the source or at a syntax error.
func (s *scanner) next() (name string, attrs map[string]string, ok bool) {
	for s.pos < len(s.src) {
		switch c := s.src[s.pos]; {
		case c == '{':
			if strings.HasPrefix(s.src[s.pos:], "{/*") {
				end := strings.Index(s.src[s.pos+3:], "*/}")
				if end < 0 {
					return "", nil, false
				}
				s.pos += 3 + end + 3
				continue
			}
			if !s.skipExpression() {
				return "", nil, false
			}
		case c == '<' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/':
			end := strings.IndexByte(s.src[s.pos:], '>')
			if end < 0 {
				return "", nil, false
			}
			s.pos += end + 1
		case c == '<':
			s.pos++
			return s.tag()
		default:
			s.pos++
		}
	}
	return "", nil, false
}

// tag reads an opening tag after its "<"
func (s *scanner) tag() (name string, attrs map[string]string, ok bool) {
	name = s.word()
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return "", nil, false
	}
	attrs = map[string]string{}
	for {
		s.space()
		if s.pos >= len(s.src) {
			return "", nil, false
		}
		switch s.src[s.pos] {
		case '>':
			s.pos++
			return name, attrs, true
		case '/':
			if !strings.HasPrefix(s.src[s.pos:], "/>") {
				return "", nil, false
			}
			s.pos += 2
			return name, attrs, true
		}
		attr := s.word()
		if attr == "" {
			return "", nil, false
		}
		s.space()
		if s.pos >= len(s.src) || s.src[s.pos] != '=' {
			continue
		}
		s.pos++
		s.space()
		if s.pos >= len(s.src) {
			return "", nil, false
		}
		switch s.src[s.pos] {
		case '"':
			end := strings.IndexAny(s.src[s.pos+1:], "\"\n")
			if end < 0 || s.src[s.pos+1+end] != '"' {
				return "", nil, false
			}
			attrs[attr] = s.src[s.pos+1 : s.pos+1+end]
			s.pos += end + 2
		case '{':
			if !s.skipExpression() {
				return "", nil, false
			}
		default:
			return "", nil, false
		}
	}
}

func (s *scanner) word() string {
	start := s.pos
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			s.pos++
			continue
		}
		break
	}
	return s.src[start:s.pos]
}

func (s *scanner) space() {
	for s.pos < len(s.src) && strings.IndexByte(" \t\n", s.src[s.pos]) >= 0 {
		s.pos++
	}
}

// skipExpression moves past the braces at the position and the expression in them, whose
// string literals may hold braces
func (s *scanner) skipExpression() bool {
	depth := 0
	for s.pos < len(s.src) {
		switch c := s.src[s.pos]; c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				s.pos++
				return true
			}
		case '\'', '"':
			s.pos++
			for s.pos < len(s.src) && s.src[s.pos] != c {
				if s.src[s.pos] == '\n' {
					return false
				}
				if s.src[s.pos] == '\\' {
					s.pos++
				}
				s.pos++
			}
			if s.pos >= len(s.src) {
				return false
			}
		}
		s.pos++
	}
	return false
}
