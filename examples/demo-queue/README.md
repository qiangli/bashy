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

From this directory, on the organizer’s device:

```sh
bashy --bashsharp ./demo-queue.bsh host
```

1. Open the printed organizer URL privately. Open the printed projector URL in a
   second browser window, move it to the projector display, and choose
   **Enable sound** and **Fullscreen**.
2. Create a fresh invite for each presenter and deliver it privately. Each
   presenter runs the following on their Outpost and pastes their invite at the
   prompt. Keep the command running during the session.

   ```sh
   bashy --bashsharp ./demo-queue.bsh join --name "Presenter"
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
