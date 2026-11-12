// SPDX-License-Identifier: GPL-3.0-only
// Derived from 1Panel commit 3adee9445897aa41c0160b4f805a9b1f43f712ac; see web/panel/manifest.json.
package flag

type Type int

const (
	EOF Type = iota
	Eol
	Keyword
	QuotedString
	Variable
	BlockStart
	BlockEnd
	Semicolon
	Comment
	Illegal
	Regex
	LuaCode
)

var (
	FlagName = map[Type]string{
		QuotedString: "QuotedString",
		EOF:          "Eof",
		Keyword:      "Keyword",
		Variable:     "Variable",
		BlockStart:   "BlockStart",
		BlockEnd:     "BlockEnd",
		Semicolon:    "Semicolon",
		Comment:      "Comment",
		Illegal:      "Illegal",
		Regex:        "Regex",
	}
)

func (tt Type) String() string {
	return FlagName[tt]
}

type Flag struct {
	Type    Type
	Literal string
	Line    int
	Column  int
}

func (t Flag) Lit(literal string) Flag {
	t.Literal = literal
	return t
}

type Flags []Flag

func (t Flag) Is(typ Type) bool {
	return t.Type == typ
}

func (t Flag) IsParameterEligible() bool {
	return t.Is(Keyword) || t.Is(QuotedString) || t.Is(Variable) || t.Is(Regex)
}
