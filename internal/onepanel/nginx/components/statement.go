// SPDX-License-Identifier: GPL-3.0-only
// Derived from 1Panel commit 3adee9445897aa41c0160b4f805a9b1f43f712ac; see web/panel/manifest.json.
package components

type IBlock interface {
	GetDirectives() []IDirective
	FindDirectives(directiveName string) []IDirective
	RemoveDirective(name string, params []string)
	UpdateDirective(name string, params []string)
	GetComment() string
	GetLine() int
	GetCodeBlock() string
}

type IDirective interface {
	GetName() string
	GetParameters() []string
	GetBlock() IBlock
	GetComment() string
	GetLine() int
}
