//go:build gomodfence

package main

import (
	"fmt"
	"github.com/fatih/color"
)

func Greeting() string { return "go fence + gomod fence" }
func main()            { fmt.Println(color.GreenString(Greeting())) }
