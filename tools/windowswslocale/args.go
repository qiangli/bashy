package main

func wslLocaleArgs(distro, locale string, localeSet bool, args []string) []string {
	wslArgs := []string{"-d", distro, "--", "env"}
	if localeSet {
		wslArgs = append(wslArgs, "LC_ALL="+locale)
	}
	wslArgs = append(wslArgs, "locale")
	return append(wslArgs, args...)
}
