// SPDX-License-Identifier: GPL-3.0-only
// Derived from 1Panel commit 3adee9445897aa41c0160b4f805a9b1f43f712ac; see web/panel/manifest.json.
package components

type Comment struct {
	Detail string
	Line   int
}

func (c *Comment) GetName() string {
	return ""
}

func (c *Comment) GetParameters() []string {
	return []string{}
}

func (c *Comment) GetBlock() IBlock {
	return nil
}

func (c *Comment) GetComment() string {
	return c.Detail
}

func (c *Comment) GetLine() int {
	return c.Line
}
