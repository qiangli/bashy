//go:build cgo && (linux || darwin)

package main

/*
#include <signal.h>
#include <stdio.h>
#include <string.h>

// C constructors run before the Go runtime installs its own signal handlers.
// Keep this snapshot in process memory; main enriches BASHY_HARD_IGNORE only
// for a shell route, after cli has saved the caller's untouched environment.
struct signal_name { int number; const char *name; };
static const struct signal_name signals[] = {
    {SIGHUP, "HUP"}, {SIGINT, "INT"}, {SIGQUIT, "QUIT"},
    {SIGILL, "ILL"}, {SIGABRT, "ABRT"}, {SIGFPE, "FPE"},
    {SIGSEGV, "SEGV"}, {SIGPIPE, "PIPE"}, {SIGALRM, "ALRM"},
    {SIGTERM, "TERM"},
#ifdef SIGBUS
    {SIGBUS, "BUS"},
#endif
#ifdef SIGUSR1
    {SIGUSR1, "USR1"},
#endif
#ifdef SIGUSR2
    {SIGUSR2, "USR2"},
#endif
#ifdef SIGTSTP
    {SIGTSTP, "TSTP"},
#endif
#ifdef SIGTTIN
    {SIGTTIN, "TTIN"},
#endif
#ifdef SIGTTOU
    {SIGTTOU, "TTOU"},
#endif
#ifdef SIGCONT
    {SIGCONT, "CONT"},
#endif
#ifdef SIGXCPU
    {SIGXCPU, "XCPU"},
#endif
#ifdef SIGXFSZ
    {SIGXFSZ, "XFSZ"},
#endif
};
static char inherited_ignored[512];

__attribute__((constructor)) static void snapshot_inherited_ignores(void) {
    for (size_t i = 0; i < sizeof signals / sizeof signals[0]; i++) {
        struct sigaction action;
        if (sigaction(signals[i].number, NULL, &action) != 0 ||
            action.sa_handler != SIG_IGN)
            continue;
        size_t used = strlen(inherited_ignored);
        size_t available = sizeof inherited_ignored - used;
        int written = snprintf(inherited_ignored + used, available, "%s%s",
                               used ? "," : "", signals[i].name);
        if (written < 0 || (size_t)written >= available) {
            inherited_ignored[used] = '\0';
            break;
        }
    }
}

static const char *pre_go_ignored_signals(void) { return inherited_ignored; }
*/
import "C"

func preGoIgnoredSignals() string { return C.GoString(C.pre_go_ignored_signals()) }
