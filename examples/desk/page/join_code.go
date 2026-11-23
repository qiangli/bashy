package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"html/template"
	"net/http"
	"regexp"
	"strings"

	"desk-kit"
)

var errJoinCodeInUse = errors.New("join code is already in use")

var joinCodePattern = regexp.MustCompile(`^(?:[a-z0-9]{4,6}|[a-z]{1,24}-[a-z]{1,24})$`)

// chooseJoinCode runs inside the store's create/update transaction.
func chooseJoinCode(raw string, used func(string) bool) (string, error) {
	code := strings.ToLower(strings.TrimSpace(raw))
	code = strings.Join(strings.Fields(code), "-")
	if code != "" {
		if !joinCodePattern.MatchString(code) {
			return "", errors.New("use two words or four to six letters or digits")
		}
		if used(code) {
			return "", errJoinCodeInUse
		}
		return code, nil
	}
	for range 128 {
		const alphabet = "23456789abcdefghjkmnpqrstuvwxyz"
		buf := make([]byte, 6)
		for i := range buf {
			buf[i] = alphabet[randomIndex(len(alphabet))]
		}
		code := string(buf)
		if !used(code) {
			return code, nil
		}
	}
	return "", errors.New("could not allocate a join code")
}

func randomIndex(n int) int {
	var b [1]byte
	for {
		rand.Read(b[:])
		if int(b[0]) < 256-256%n {
			return int(b[0]) % n
		}
	}
}

func pageJoinCode(path string) string {
	code, ok := strings.CutPrefix(strings.TrimSuffix(path, "/"), "/")
	if ok && joinCodePattern.MatchString(code) {
		return code
	}
	return ""
}

var joinScreen = template.Must(template.New("join").Parse(`<main class="desk-main desk-narrow">
<h1>{{.Title}}</h1>
{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form method="post" action="{{.Action}}" class="desk-panel">
{{if .Page}}<label>Name or email <input name="handle" maxlength="80" autocomplete="nickname" required></label>
<button type="submit">Join</button>
<button type="submit" name="random" value="1" formnovalidate>Use a random handle</button>
{{else}}<label>Join code <input name="code" maxlength="49" autocomplete="off" required></label>
<button type="submit">Continue</button>{{end}}
</form></main>`))

type joinScreenData struct {
	Page, Error, Action, Title, Home string
}

func writeJoinScreen(w http.ResponseWriter, status int, data joinScreenData) {
	data.Title = "Join a page"
	if data.Page != "" {
		data.Title = "Join " + data.Page
	}
	var body bytes.Buffer
	if err := joinScreen.Execute(&body, data); err != nil {
		http.Error(w, "Could not render join screen", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(kit.Page("page", data.Title, kit.AppBar("page", data.Home, kit.EmptyItemHeader("page"), "")+body.String()))
}
