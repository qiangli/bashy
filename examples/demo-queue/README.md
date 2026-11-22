# Demo Queue

Take timed screen-sharing turns on a projector through Outpost. Presenters can
show an interactive browser session, an application window, or a whole display.
To play a video with sound, open it in a Chrome or Edge tab and select that tab
with audio enabled. The projector plays the shared media. Viewers use their
own keyboards and mice.

## Run a session

Use current desktop Google Chrome or Microsoft Edge, Go 1.27+, Bashy, and a
running paired Outpost with its mesh enabled on each device. Use the same LAN
and check that the organizer and presenters appear with a direct connection in
`outpost mesh status`. The source works under the repository's BSD-3-Clause
license and has zero third-party library dependencies.

Build once on each device from this directory:

```sh
bashy --bashsharp ./demo-queue.bsh build
```

Run the native binary from your terminal so it receives Ctrl-C and can close
its Outpost forwards. Use `.build/demo-queue.exe` on Windows.

```sh
./.build/demo-queue host
```

1. Open the printed organizer URL privately. Open the printed projector URL in a
   second browser window, move it to the projector display, and choose
   **Enable sound** and **Fullscreen**.
2. Create a fresh invite for each presenter and deliver it privately. Each
   presenter runs the following on their Outpost and pastes their invite at the
   prompt. Keep the command running during the session.

   ```sh
   ./.build/demo-queue join --name "Presenter"
   ```

3. The presenter opens the printed local URL and clicks **Prepare to present**.
   Choose the tab, window, or display and keep that source open. The local
   preview stays on the presenter’s device. A green **Ready** label means the
   source and direct mesh upload checks passed. Use **Leave queue** to change
   sources and prepare again.
4. Set the turn length and warning, then select **Start**. The defaults are 120
   seconds and a 15-second warning. Ready presenters share automatically in ready
   order. The timer starts when their first video frame appears.
5. Use **Skip** to advance, **Assign turn** to change or recall a presenter, and
   **End session** to finish. A recalled presenter may need to prepare again.
   Stop the host and join commands with Ctrl-C after the session to remove
   their temporary mesh routes.

If a connection check fails, confirm the Outpost link is direct and retry. If a
browser cannot open the capture picker, check its screen-capture permission
and the OS screen-sharing setting. If video is silent, verify that the speaker
output is connected to the projector and that tab audio was enabled in the
browser picker. Restarting the host starts a fresh session.

## Verify

```sh
bashy --bashsharp ./demo-queue.bsh test
```

For an in-room rehearsal, use one organizer Outpost and two presenter Outposts
on the same LAN. Ask a Chrome presenter to prepare a tab playing a 30-second
video with audio and an Edge presenter to prepare a tab with an interactive
browser session. Check both green Ready labels. Start the queue and confirm
the video and its audio stay in sync, the countdown starts when it appears,
and the outgoing audio stops as the interactive session takes over. Try
skipping and recalling a presenter. Repeat with the other projector browser
and confirm that closing a presenter command clears its readiness.

The Sprint 690 automated rehearsal used desktop Chrome and Edge 158 with
three distinct Outpost mesh peers on one machine. Both projector browsers
decoded over 30 seconds of shared tab video and audio, then displayed an
interactive browser tab on the next turn. The installed Outpost CLI also
passed the host expose and cleanup smoke check. The in-room rehearsal above
checks the physical projector, speakers, and separate LAN devices.

## Agent CLI

Agents can run the organizer, presenter, and projector roles through the same
session endpoints as the browser. Commands accept `--url PRIVATE_LINK` or
`DEMO_QUEUE_URL`. Keep the role links private. Control actions return JSON.
Use `--dry-run` on writes to check a proposed action without committing it.

| Browser action | Native CLI action |
|---|---|
| Host a session and open its role links | `host --json`, then `links` |
| Create or copy a private invite | `invite --json` returns the invite |
| Join with an invite and display name | `join --name NAME --invite-file PATH --json` |
| Inspect the queue, readiness, and countdown | `status --json` or `status --watch` |
| Start with timing settings | `start --limit 120 --warning 15` |
| Assign, skip, or end | `assign --id ID`, `skip`, or `end` |
| Prepare and share when called | `prepare --input CAPTURE.webm --audio` |
| Leave the ready queue or finish the turn | `leave` or `finish` |
| Set projector sound and fullscreen layout | `display --sound on --fullscreen on` |
| Connect a projector and consume the current turn | `view > turn.webm`, or `viewer`, `chunk`, and `rendered` for a player that acknowledges its own first frame |

An organizer agent runs the host and controls from the terminal. A presenter
agent supplies an authorized VP8 WebM source as a file or as live stdin with
`--input -`. Add `--audio` for an Opus track. The CLI checks the input and Outpost
connection first, waits in the ready queue, and starts publishing automatically
when called. A browser share uses its native capture picker and local preview.
The presenter chooses which desktop, window, or tab the browser may capture.
A terminal projector agent connects `view` to its playback sink. The sink
controls its speakers and fullscreen presentation. The browser can enter OS
fullscreen from its local button when browser policy requires a gesture.

## Shape the app

```sh
bash ./register.sh
bashy app show demo-queue --json
```

Open **Demo Queue** through the Bashy app console to use Genie. Paste a private
role link from the native process running on the console's machine. The
shaped page uses the same live session and retains the role's permissions.
For agents, `links --via-shape` prints the corresponding console URL.
Add `--via-shape` to other CLI actions to use that principal's running console
instance. Open the app in the console first.

Use `bashy shape wish demo-queue`, `shape verify`, `shape preview`,
`shape approve`, and `shape undo` for a personal revision. The candidate
build and tests are declared in `contract.bsh` and `checks/go.bsh`.

## Native automated browser tests

Install the test-only Apache-2.0 Playwright package and desktop Chrome and Edge,
then run the suite from this directory. The example binary has no third-party
runtime libraries.

```sh
npm install --no-save --no-package-lock playwright@1.65.1
node checks/e2e.mjs
```

The suite uses an isolated Outpost command fixture and a generated capture
source. Chrome and Edge use their real VP8/Opus encoders and decoders. It drives
browser and terminal presenters, a terminal organizer and projector, timed
handoff, and the shaped session link. The opt-in GitHub workflow runs the same
suite on macOS and Windows. The in-room check exercises the OS picker, separate
Outposts, physical projector, and speakers.
