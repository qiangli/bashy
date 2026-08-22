# Interactive `sh` startup compatibility plan

1. Pin the observed Profile B `fc` terminal hang with a PTY test for the
   interactive `sh` ENV handshake and historical default prompt.
2. Match GNU Bash 5.3 startup boundaries for `sh`, login shells, explicit
   POSIX mode, and non-interactive execution without changing ordinary Bash
   startup behavior.
3. Run focused, race, full, cross-build, and isolated targeted `fc` gates
   before integrating the provider change or starting a fresh full arm.
