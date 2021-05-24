package main

import (
	"fmt"
	"regexp"
)

// Cloak 隐身模式
type Cloak struct {
	reg *regexp.Regexp
	to  string
}

// Replace 替换字符中的id为隐身后的id
func (c *Cloak) Replace(buf []byte) []byte {
	return []byte(c.reg.ReplaceAllString(string(buf), c.to))
}

func newCloak(id string, to string) (*Cloak, error) {
	r := fmt.Sprintf(`(?i)\[36m%s`, id)
	t := fmt.Sprintf(`[36m%s`, to)
	reg, err := regexp.Compile(r)
	if err != nil {
		return nil, err
	}

	return &Cloak{
		reg: reg,
		to:  t,
	}, nil
}
