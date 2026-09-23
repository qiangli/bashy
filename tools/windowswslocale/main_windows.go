package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	distro := os.Getenv("BASHY_WSL_DISTRO")
	if distro == "" {
		fmt.Fprintln(os.Stderr, "wsl locale provider: BASHY_WSL_DISTRO is unset")
		os.Exit(2)
	}
	locale, localeSet := os.LookupEnv("LC_ALL")
	wsl := os.Getenv("BASHY_WSL_EXE")
	if wsl == "" {
		wsl = "wsl.exe"
	}
	cmd := exec.Command(wsl, wslLocaleArgs(distro, locale, localeSet, os.Args[1:])...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if failed, ok := err.(*exec.ExitError); ok {
			os.Exit(failed.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "wsl locale provider:", err)
		os.Exit(1)
	}
}
