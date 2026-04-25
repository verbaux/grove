package main

import (
	_ "embed"

	"github.com/verbaux/grove/cmd"
)

//go:embed skills/grove/SKILL.md
var skillContent string

func init() {
	cmd.SkillContent = skillContent
}
