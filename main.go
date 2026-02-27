package main

import "github.com/verbaux/grove/cmd"

var version = "dev"

func main() {
	cmd.Version = version
	cmd.Execute()
}
