package data

import (
	"strings"
	"fmt"
	"math"
	"sort"
	"strconv"
)

const endSymbol rune = 1114112

/* The rule types inferred from the grammar are below. */
type pegRule uint8

const (
	ruleUnknown pegRule = iota
	rulejsonPath
	rulejsonPathHead
	rulejsonPathNonHead
	rulejsonMapSingleLevel
	rulejsonMapMultipleLevel
	rulejsonMapAccessString
	rulejsonMapAccessBracket
	rulesingleQuotedString
	ruledoubleQuotedString
	rulejsonArrayAccess
	rulejsonArraySlice
	rulejsonArrayPartialSlice
	rulejsonArrayFullSlice
	ruleAction0
	ruleAction1
	ruleAction2
	rulePegText
	ruleAction3
	ruleAction4
	ruleAction5
	ruleAction6
	ruleAction7
	ruleAction8
	ruleAction9
)

var rul3s = [...]string{
	"Unknown",
	"jsonPath",
	"jsonPathHead",
	"jsonPathNonHead",
	"jsonMapSingleLevel",
	"jsonMapMultipleLevel",
	"jsonMapAccessString",
	"jsonMapAccessBracket",
	"singleQuotedString",
	"doubleQuotedString",
	"jsonArrayAccess",
	"jsonArraySlice",
	"jsonArrayPartialSlice",
	"jsonArrayFullSlice",
	"Action0",
	"Action1",
	"Action2",
	"PegText",
	"Action3",
	"Action4",
	"Action5",
	"Action6",
	"Action7",
	"Action8",
	"Action9",
}

type token32 struct {
	pegRule
	begin, end uint32
}

func (t *token32) String() string {
	return fmt.Sprintf("\x1B[34m%v\x1B[m %v %v", rul3s[t.pegRule], t.begin, t.end)
}

type node32 struct {
	token32
	up, next *node32
}

func (node *node32) print(pretty bool, buffer string) {
	var print func(node *node32, depth int)
	print = func(node *node32, depth int) {
		for node != nil {
			for c := 0; c < depth; c++ {
				fmt.Printf(" ")
			}
			rule := rul3s[node.pegRule]
			quote := strconv.Quote(string(([]rune(buffer)[node.begin:node.end])))
			if !pretty {
				fmt.Printf("%v %v\n", rule, quote)
			} else {
				fmt.Printf("\x1B[34m%v\x1B[m %v\n", rule, quote)
			}
			if node.up != nil {
				print(node.up, depth+1)
			}
			node = node.next
		}
	}
	print(node, 0)
}

func (node *node32) Print(buffer string) {
	node.print(false, buffer)
}

func (node *node32) PrettyPrint(buffer string) {
	node.print(true, buffer)
}

type tokens32 struct {
	tree []token32
}

func (t *tokens32) Trim(length uint32) {
	t.tree = t.tree[:length]
}

func (t *tokens32) Print() {
	for _, token := range t.tree {
		fmt.Println(token.String())
	}
}

func (t *tokens32) AST() *node32 {
	type element struct {
		node *node32
		down *element
	}
	tokens := t.Tokens()
	var stack *element
	for _, token := range tokens {
		if token.begin == token.end {
			continue
		}
		node := &node32{token32: token}
		for stack != nil && stack.node.begin >= token.begin && stack.node.end <= token.end {
			stack.node.next = node.up
			node.up = stack.node
			stack = stack.down
		}
		stack = &element{node: node, down: stack}
	}
	if stack != nil {
		return stack.node
	}
	return nil
}

func (t *tokens32) PrintSyntaxTree(buffer string) {
	t.AST().Print(buffer)
}

func (t *tokens32) PrettyPrintSyntaxTree(buffer string) {
	t.AST().PrettyPrint(buffer)
}

func (t *tokens32) Add(rule pegRule, begin, end, index uint32) {
	if tree := t.tree; int(index) >= len(tree) {
		expanded := make([]token32, 2*len(tree))
		copy(expanded, tree)
		t.tree = expanded
	}
	t.tree[index] = token32{
		pegRule: rule,
		begin:   begin,
		end:     end,
	}
}

func (t *tokens32) Tokens() []token32 {
	return t.tree
}

type jsonPeg struct {
	components []extractor
	lastKey    string

	Buffer string
	buffer []rune
	rules  [25]func() bool
	parse  func(rule ...int) error
	reset  func()
	Pretty bool
	tokens32
}

func (p *jsonPeg) Parse(rule ...int) error {
	return p.parse(rule...)
}

func (p *jsonPeg) Reset() {
	p.reset()
}

type textPosition struct {
	line, symbol int
}

type textPositionMap map[int]textPosition

func translatePositions(buffer []rune, positions []int) textPositionMap {
	length, translations, j, line, symbol := len(positions), make(textPositionMap, len(positions)), 0, 1, 0
	sort.Ints(positions)

search:
	for i, c := range buffer {
		if c == '\n' {
			line, symbol = line+1, 0
		} else {
			symbol++
		}
		if i == positions[j] {
			translations[positions[j]] = textPosition{line, symbol}
			for j++; j < length; j++ {
				if i != positions[j] {
					continue search
				}
			}
			break search
		}
	}

	return translations
}

type parseError struct {
	p   *jsonPeg
	max token32
}

func (e *parseError) Error() string {
	tokens, error := []token32{e.max}, "\n"
	positions, p := make([]int, 2*len(tokens)), 0
	for _, token := range tokens {
		positions[p], p = int(token.begin), p+1
		positions[p], p = int(token.end), p+1
	}
	translations := translatePositions(e.p.buffer, positions)
	format := "parse error near %v (line %v symbol %v - line %v symbol %v):\n%v\n"
	if e.p.Pretty {
		format = "parse error near \x1B[34m%v\x1B[m (line %v symbol %v - line %v symbol %v):\n%v\n"
	}
	for _, token := range tokens {
		begin, end := int(token.begin), int(token.end)
		error += fmt.Sprintf(format,
			rul3s[token.pegRule],
			translations[begin].line, translations[begin].symbol,
			translations[end].line, translations[end].symbol,
			strconv.Quote(string(e.p.buffer[begin:end])))
	}

	return error
}

func (p *jsonPeg) PrintSyntaxTree() {
	if p.Pretty {
		p.tokens32.PrettyPrintSyntaxTree(p.Buffer)
	} else {
		p.tokens32.PrintSyntaxTree(p.Buffer)
	}
}

func (p *jsonPeg) Execute() {
	buffer, _buffer, text, begin, end := p.Buffer, p.buffer, "", 0, 0
	for _, token := range p.Tokens() {
		switch token.pegRule {

		case rulePegText:
			begin, end = int(token.begin), int(token.end)
			text = string(_buffer[begin:end])

		case ruleAction0:

			p.addMapAccess(p.lastKey)

		case ruleAction1:

			p.addMapAccess(p.lastKey)

		case ruleAction2:

			p.addRecursiveAccess(p.lastKey)

		case ruleAction3:

			substr := string([]rune(buffer)[begin:end])
			p.lastKey = substr

		case ruleAction4:

			substr := string([]rune(buffer)[begin:end])
			p.lastKey = strings.Replace(substr, "''", "'", -1)

		case ruleAction5:

			substr := string([]rune(buffer)[begin:end])
			p.lastKey = strings.Replace(substr, "\"\"", "\"", -1)

		case ruleAction6:

			substr := string([]rune(buffer)[begin:end])
			p.addArrayAccess(substr)

		case ruleAction7:

			substr := string([]rune(buffer)[begin:end])
			p.addArraySlice(substr)

		case ruleAction8:

			substr := string([]rune(buffer)[begin:end])
			p.addArraySlice(substr)

		case ruleAction9:

			p.addArraySlice("0:")

		}
	}
	_, _, _, _, _ = buffer, _buffer, text, begin, end
}

func (p *jsonPeg) Init() {
	var (
		max                  token32
		position, tokenIndex uint32
		buffer               []rune
	)
	p.reset = func() {
		max = token32{}
		position, tokenIndex = 0, 0

		p.buffer = []rune(p.Buffer)
		if len(p.buffer) == 0 || p.buffer[len(p.buffer)-1] != endSymbol {
			p.buffer = append(p.buffer, endSymbol)
		}
		buffer = p.buffer
	}
	p.reset()

	_rules := p.rules
	tree := tokens32{tree: make([]token32, math.MaxInt16)}
	p.parse = func(rule ...int) error {
		r := 1
		if len(rule) > 0 {
			r = rule[0]
		}
		matches := p.rules[r]()
		p.tokens32 = tree
		if matches {
			p.Trim(tokenIndex)
			return nil
		}
		return &parseError{p, max}
	}

	add := func(rule pegRule, begin uint32) {
		tree.Add(rule, begin, position, tokenIndex)
		tokenIndex++
		if begin != position && position > max.end {
			max = token32{rule, begin, position}
		}
	}

	matchDot := func() bool {
		if buffer[position] != endSymbol {
			position++
			return true
		}
		return false
	}

	/*matchChar := func(c byte) bool {
		if buffer[position] == c {
			position++
			return true
		}
		return false
	}*/

	/*matchRange := func(lower byte, upper byte) bool {
		if c := buffer[position]; c >= lower && c <= upper {
			position++
			return true
		}
		return false
	}*/

	_rules = [...]func() bool{
		nil,
		/* 0 jsonPath <- <(jsonPathHead? jsonPathNonHead* !.)> */
		func() bool {
			position0, tokenIndex0 := position, tokenIndex
			{
				position1 := position
				{
					position2, tokenIndex2 := position, tokenIndex
					if !_rules[rulejsonPathHead]() {
						goto l2
					}
					goto l3
				l2:
					position, tokenIndex = position2, tokenIndex2
				}
			l3:
			l4:
				{
					position5, tokenIndex5 := position, tokenIndex
					if !_rules[rulejsonPathNonHead]() {
						goto l5
					}
					goto l4
				l5:
					position, tokenIndex = position5, tokenIndex5
				}
				{
					position6, tokenIndex6 := position, tokenIndex
					if !matchDot() {
						goto l6
					}
					goto l0
				l6:
					position, tokenIndex = position6, tokenIndex6
				}
				add(rulejsonPath, position1)
			}
			return true
		l0:
			position, tokenIndex = position0, tokenIndex0
			return false
		},
		/* 1 jsonPathHead <- <((jsonMapAccessString / jsonMapAccessBracket) Action0)> */
		func() bool {
			position7, tokenIndex7 := position, tokenIndex
			{
				position8 := position
				{
					position9, tokenIndex9 := position, tokenIndex
					if !_rules[rulejsonMapAccessString]() {
						goto l10
					}
					goto l9
				l10:
					position, tokenIndex = position9, tokenIndex9
					if !_rules[rulejsonMapAccessBracket]() {
						goto l7
					}
				}
			l9:
				if !_rules[ruleAction0]() {
					goto l7
				}
				add(rulejsonPathHead, position8)
			}
			return true
		l7:
			position, tokenIndex = position7, tokenIndex7
			return false
		},
		/* 2 jsonPathNonHead <- <(jsonMapMultipleLevel / jsonMapSingleLevel / jsonArrayFullSlice / jsonArrayPartialSlice / jsonArraySlice / jsonArrayAccess)> */
		func() bool {
			position11, tokenIndex11 := position, tokenIndex
			{
				position12 := position
				{
					position13, tokenIndex13 := position, tokenIndex
					if !_rules[rulejsonMapMultipleLevel]() {
						goto l14
					}
					goto l13
				l14:
					position, tokenIndex = position13, tokenIndex13
					if !_rules[rulejsonMapSingleLevel]() {
						goto l15
					}
					goto l13
				l15:
					position, tokenIndex = position13, tokenIndex13
					if !_rules[rulejsonArrayFullSlice]() {
						goto l16
					}
					goto l13
				l16:
					position, tokenIndex = position13, tokenIndex13
					if !_rules[rulejsonArrayPartialSlice]() {
						goto l17
					}
					goto l13
				l17:
					position, tokenIndex = position13, tokenIndex13
					if !_rules[rulejsonArraySlice]() {
						goto l18
					}
					goto l13
				l18:
					position, tokenIndex = position13, tokenIndex13
					if !_rules[rulejsonArrayAccess]() {
						goto l11
					}
				}
			l13:
				add(rulejsonPathNonHead, position12)
			}
			return true
		l11:
			position, tokenIndex = position11, tokenIndex11
			return false
		},
		/* 3 jsonMapSingleLevel <- <((('.' jsonMapAccessString) / jsonMapAccessBracket) Action1)> */
		func() bool {
			position19, tokenIndex19 := position, tokenIndex
			{
				position20 := position
				{
					position21, tokenIndex21 := position, tokenIndex
					if buffer[position] != rune('.') {
						goto l22
					}
					position++
					if !_rules[rulejsonMapAccessString]() {
						goto l22
					}
					goto l21
				l22:
					position, tokenIndex = position21, tokenIndex21
					if !_rules[rulejsonMapAccessBracket]() {
						goto l19
					}
				}
			l21:
				if !_rules[ruleAction1]() {
					goto l19
				}
				add(rulejsonMapSingleLevel, position20)
			}
			return true
		l19:
			position, tokenIndex = position19, tokenIndex19
			return false
		},
		/* 4 jsonMapMultipleLevel <- <('.' '.' (jsonMapAccessString / jsonMapAccessBracket) Action2)> */
		func() bool {
			position23, tokenIndex23 := position, tokenIndex
			{
				position24 := position
				if buffer[position] != rune('.') {
					goto l23
				}
				position++
				if buffer[position] != rune('.') {
					goto l23
				}
				position++
				{
					position25, tokenIndex25 := position, tokenIndex
					if !_rules[rulejsonMapAccessString]() {
						goto l26
					}
					goto l25
				l26:
					position, tokenIndex = position25, tokenIndex25
					if !_rules[rulejsonMapAccessBracket]() {
						goto l23
					}
				}
			l25:
				if !_rules[ruleAction2]() {
					goto l23
				}
				add(rulejsonMapMultipleLevel, position24)
			}
			return true
		l23:
			position, tokenIndex = position23, tokenIndex23
			return false
		},
		/* 5 jsonMapAccessString <- <(<(([a-z] / [A-Z]) ([a-z] / [A-Z] / [0-9] / '_')*)> Action3)> */
		func() bool {
			position27, tokenIndex27 := position, tokenIndex
			{
				position28 := position
				{
					position29 := position
					{
						position30, tokenIndex30 := position, tokenIndex
						if c := buffer[position]; c < rune('a') || c > rune('z') {
							goto l31
						}
						position++
						goto l30
					l31:
						position, tokenIndex = position30, tokenIndex30
						if c := buffer[position]; c < rune('A') || c > rune('Z') {
							goto l27
						}
						position++
					}
				l30:
				l32:
					{
						position33, tokenIndex33 := position, tokenIndex
						{
							position34, tokenIndex34 := position, tokenIndex
							if c := buffer[position]; c < rune('a') || c > rune('z') {
								goto l35
							}
							position++
							goto l34
						l35:
							position, tokenIndex = position34, tokenIndex34
							if c := buffer[position]; c < rune('A') || c > rune('Z') {
								goto l36
							}
							position++
							goto l34
						l36:
							position, tokenIndex = position34, tokenIndex34
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l37
							}
							position++
							goto l34
						l37:
							position, tokenIndex = position34, tokenIndex34
							if buffer[position] != rune('_') {
								goto l33
							}
							position++
						}
					l34:
						goto l32
					l33:
						position, tokenIndex = position33, tokenIndex33
					}
					add(rulePegText, position29)
				}
				if !_rules[ruleAction3]() {
					goto l27
				}
				add(rulejsonMapAccessString, position28)
			}
			return true
		l27:
			position, tokenIndex = position27, tokenIndex27
			return false
		},
		/* 6 jsonMapAccessBracket <- <('[' (singleQuotedString / doubleQuotedString) ']')> */
		func() bool {
			position38, tokenIndex38 := position, tokenIndex
			{
				position39 := position
				if buffer[position] != rune('[') {
					goto l38
				}
				position++
				{
					position40, tokenIndex40 := position, tokenIndex
					if !_rules[rulesingleQuotedString]() {
						goto l41
					}
					goto l40
				l41:
					position, tokenIndex = position40, tokenIndex40
					if !_rules[ruledoubleQuotedString]() {
						goto l38
					}
				}
			l40:
				if buffer[position] != rune(']') {
					goto l38
				}
				position++
				add(rulejsonMapAccessBracket, position39)
			}
			return true
		l38:
			position, tokenIndex = position38, tokenIndex38
			return false
		},
		/* 7 singleQuotedString <- <('\'' <(('\'' '\'') / (!'\'' .))*> '\'' Action4)> */
		func() bool {
			position42, tokenIndex42 := position, tokenIndex
			{
				position43 := position
				if buffer[position] != rune('\'') {
					goto l42
				}
				position++
				{
					position44 := position
				l45:
					{
						position46, tokenIndex46 := position, tokenIndex
						{
							position47, tokenIndex47 := position, tokenIndex
							if buffer[position] != rune('\'') {
								goto l48
							}
							position++
							if buffer[position] != rune('\'') {
								goto l48
							}
							position++
							goto l47
						l48:
							position, tokenIndex = position47, tokenIndex47
							{
								position49, tokenIndex49 := position, tokenIndex
								if buffer[position] != rune('\'') {
									goto l49
								}
								position++
								goto l46
							l49:
								position, tokenIndex = position49, tokenIndex49
							}
							if !matchDot() {
								goto l46
							}
						}
					l47:
						goto l45
					l46:
						position, tokenIndex = position46, tokenIndex46
					}
					add(rulePegText, position44)
				}
				if buffer[position] != rune('\'') {
					goto l42
				}
				position++
				if !_rules[ruleAction4]() {
					goto l42
				}
				add(rulesingleQuotedString, position43)
			}
			return true
		l42:
			position, tokenIndex = position42, tokenIndex42
			return false
		},
		/* 8 doubleQuotedString <- <('"' <(('"' '"') / (!'"' .))*> '"' Action5)> */
		func() bool {
			position50, tokenIndex50 := position, tokenIndex
			{
				position51 := position
				if buffer[position] != rune('"') {
					goto l50
				}
				position++
				{
					position52 := position
				l53:
					{
						position54, tokenIndex54 := position, tokenIndex
						{
							position55, tokenIndex55 := position, tokenIndex
							if buffer[position] != rune('"') {
								goto l56
							}
							position++
							if buffer[position] != rune('"') {
								goto l56
							}
							position++
							goto l55
						l56:
							position, tokenIndex = position55, tokenIndex55
							{
								position57, tokenIndex57 := position, tokenIndex
								if buffer[position] != rune('"') {
									goto l57
								}
								position++
								goto l54
							l57:
								position, tokenIndex = position57, tokenIndex57
							}
							if !matchDot() {
								goto l54
							}
						}
					l55:
						goto l53
					l54:
						position, tokenIndex = position54, tokenIndex54
					}
					add(rulePegText, position52)
				}
				if buffer[position] != rune('"') {
					goto l50
				}
				position++
				if !_rules[ruleAction5]() {
					goto l50
				}
				add(ruledoubleQuotedString, position51)
			}
			return true
		l50:
			position, tokenIndex = position50, tokenIndex50
			return false
		},
		/* 9 jsonArrayAccess <- <('[' <('-'? [0-9]+)> ']' Action6)> */
		func() bool {
			position58, tokenIndex58 := position, tokenIndex
			{
				position59 := position
				if buffer[position] != rune('[') {
					goto l58
				}
				position++
				{
					position60 := position
					{
						position61, tokenIndex61 := position, tokenIndex
						if buffer[position] != rune('-') {
							goto l61
						}
						position++
						goto l62
					l61:
						position, tokenIndex = position61, tokenIndex61
					}
				l62:
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l58
					}
					position++
				l63:
					{
						position64, tokenIndex64 := position, tokenIndex
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l64
						}
						position++
						goto l63
					l64:
						position, tokenIndex = position64, tokenIndex64
					}
					add(rulePegText, position60)
				}
				if buffer[position] != rune(']') {
					goto l58
				}
				position++
				if !_rules[ruleAction6]() {
					goto l58
				}
				add(rulejsonArrayAccess, position59)
			}
			return true
		l58:
			position, tokenIndex = position58, tokenIndex58
			return false
		},
		/* 10 jsonArraySlice <- <('[' <('-'? [0-9]+ ':' '-'? [0-9]+ (':' '-'? [0-9]+)?)> ']' Action7)> */
		func() bool {
			position65, tokenIndex65 := position, tokenIndex
			{
				position66 := position
				if buffer[position] != rune('[') {
					goto l65
				}
				position++
				{
					position67 := position
					{
						position68, tokenIndex68 := position, tokenIndex
						if buffer[position] != rune('-') {
							goto l68
						}
						position++
						goto l69
					l68:
						position, tokenIndex = position68, tokenIndex68
					}
				l69:
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l65
					}
					position++
				l70:
					{
						position71, tokenIndex71 := position, tokenIndex
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l71
						}
						position++
						goto l70
					l71:
						position, tokenIndex = position71, tokenIndex71
					}
					if buffer[position] != rune(':') {
						goto l65
					}
					position++
					{
						position72, tokenIndex72 := position, tokenIndex
						if buffer[position] != rune('-') {
							goto l72
						}
						position++
						goto l73
					l72:
						position, tokenIndex = position72, tokenIndex72
					}
				l73:
					if c := buffer[position]; c < rune('0') || c > rune('9') {
						goto l65
					}
					position++
				l74:
					{
						position75, tokenIndex75 := position, tokenIndex
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l75
						}
						position++
						goto l74
					l75:
						position, tokenIndex = position75, tokenIndex75
					}
					{
						position76, tokenIndex76 := position, tokenIndex
						if buffer[position] != rune(':') {
							goto l76
						}
						position++
						{
							position78, tokenIndex78 := position, tokenIndex
							if buffer[position] != rune('-') {
								goto l78
							}
							position++
							goto l79
						l78:
							position, tokenIndex = position78, tokenIndex78
						}
					l79:
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l76
						}
						position++
					l80:
						{
							position81, tokenIndex81 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l81
							}
							position++
							goto l80
						l81:
							position, tokenIndex = position81, tokenIndex81
						}
						goto l77
					l76:
						position, tokenIndex = position76, tokenIndex76
					}
				l77:
					add(rulePegText, position67)
				}
				if buffer[position] != rune(']') {
					goto l65
				}
				position++
				if !_rules[ruleAction7]() {
					goto l65
				}
				add(rulejsonArraySlice, position66)
			}
			return true
		l65:
			position, tokenIndex = position65, tokenIndex65
			return false
		},
		/* 11 jsonArrayPartialSlice <- <('[' <((':' '-'? [0-9]+) / ('-'? [0-9]+ ':'))> ']' Action8)> */
		func() bool {
			position82, tokenIndex82 := position, tokenIndex
			{
				position83 := position
				if buffer[position] != rune('[') {
					goto l82
				}
				position++
				{
					position84 := position
					{
						position85, tokenIndex85 := position, tokenIndex
						if buffer[position] != rune(':') {
							goto l86
						}
						position++
						{
							position87, tokenIndex87 := position, tokenIndex
							if buffer[position] != rune('-') {
								goto l87
							}
							position++
							goto l88
						l87:
							position, tokenIndex = position87, tokenIndex87
						}
					l88:
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l86
						}
						position++
					l89:
						{
							position90, tokenIndex90 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l90
							}
							position++
							goto l89
						l90:
							position, tokenIndex = position90, tokenIndex90
						}
						goto l85
					l86:
						position, tokenIndex = position85, tokenIndex85
						{
							position91, tokenIndex91 := position, tokenIndex
							if buffer[position] != rune('-') {
								goto l91
							}
							position++
							goto l92
						l91:
							position, tokenIndex = position91, tokenIndex91
						}
					l92:
						if c := buffer[position]; c < rune('0') || c > rune('9') {
							goto l82
						}
						position++
					l93:
						{
							position94, tokenIndex94 := position, tokenIndex
							if c := buffer[position]; c < rune('0') || c > rune('9') {
								goto l94
							}
							position++
							goto l93
						l94:
							position, tokenIndex = position94, tokenIndex94
						}
						if buffer[position] != rune(':') {
							goto l82
						}
						position++
					}
				l85:
					add(rulePegText, position84)
				}
				if buffer[position] != rune(']') {
					goto l82
				}
				position++
				if !_rules[ruleAction8]() {
					goto l82
				}
				add(rulejsonArrayPartialSlice, position83)
			}
			return true
		l82:
			position, tokenIndex = position82, tokenIndex82
			return false
		},
		/* 12 jsonArrayFullSlice <- <('[' ':' ']' Action9)> */
		func() bool {
			position95, tokenIndex95 := position, tokenIndex
			{
				position96 := position
				if buffer[position] != rune('[') {
					goto l95
				}
				position++
				if buffer[position] != rune(':') {
					goto l95
				}
				position++
				if buffer[position] != rune(']') {
					goto l95
				}
				position++
				if !_rules[ruleAction9]() {
					goto l95
				}
				add(rulejsonArrayFullSlice, position96)
			}
			return true
		l95:
			position, tokenIndex = position95, tokenIndex95
			return false
		},
		/* 14 Action0 <- <{
		    p.addMapAccess(p.lastKey)
		}> */
		func() bool {
			{
				add(ruleAction0, position)
			}
			return true
		},
		/* 15 Action1 <- <{
		    p.addMapAccess(p.lastKey)
		}> */
		func() bool {
			{
				add(ruleAction1, position)
			}
			return true
		},
		/* 16 Action2 <- <{
		    p.addRecursiveAccess(p.lastKey)
		}> */
		func() bool {
			{
				add(ruleAction2, position)
			}
			return true
		},
		nil,
		/* 18 Action3 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.lastKey = substr
		}> */
		func() bool {
			{
				add(ruleAction3, position)
			}
			return true
		},
		/* 19 Action4 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.lastKey = strings.Replace(substr, "''", "'", -1)
		}> */
		func() bool {
			{
				add(ruleAction4, position)
			}
			return true
		},
		/* 20 Action5 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.lastKey = strings.Replace(substr, "\"\"", "\"", -1)
		}> */
		func() bool {
			{
				add(ruleAction5, position)
			}
			return true
		},
		/* 21 Action6 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.addArrayAccess(substr)
		}> */
		func() bool {
			{
				add(ruleAction6, position)
			}
			return true
		},
		/* 22 Action7 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.addArraySlice(substr)
		}> */
		func() bool {
			{
				add(ruleAction7, position)
			}
			return true
		},
		/* 23 Action8 <- <{
		    substr := string([]rune(buffer)[begin:end])
		    p.addArraySlice(substr)
		}> */
		func() bool {
			{
				add(ruleAction8, position)
			}
			return true
		},
		/* 24 Action9 <- <{
		    p.addArraySlice("0:")
		}> */
		func() bool {
			{
				add(ruleAction9, position)
			}
			return true
		},
	}
	p.rules = _rules
}
