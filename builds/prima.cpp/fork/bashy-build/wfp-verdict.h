/* TCP negative-control verdict classifier for the prima.cpp no-egress WFP
 * gate. Deliberately free of Windows headers so the classifier compiles and
 * unit-tests anywhere (check_test.py builds a harness with the host gcc).
 *
 * CI run 38472050604 (tag 0.1.2-8): the deliberate external TCP 443 connect
 * to unroutable TEST-NET-1 neither got an explicit deny nor connected --
 * select() just timed out. A BLOCK at FWPM_LAYER_ALE_AUTH_CONNECT_V4
 * surfaces as WSAEACCES (synchronously, or via SO_ERROR after a writable
 * select), never as silence: silence means the SYN left the host and no
 * answer came back, i.e. either our filter never matched the flow
 * (app-id/condition mismatch, runner fabric drop) or a below-ALE drop
 * swallowed it without a verdict. Retrying the silence cannot tell those
 * apart, so the gate classifies the single attempt (below) and accepts a
 * silent drop for the external control ONLY when a CLASSIFY_DROP net event
 * proves OUR filter matched the flow (see windows-net-guard.c). The real
 * exercise stays strict: any observed denial past the marker fails.
 */
#ifndef BASHY_WFP_VERDICT_H
#define BASHY_WFP_VERDICT_H

/* Winsock error values (WSAGetLastError / SO_ERROR domain). Duplicated here
 * so non-Windows test builds compile; windows-net-guard.c _Static_asserts
 * each against the real Winsock constant, so drift fails the Windows build
 * instead of silently forking the classifier. */
#define WFP_WSAEACCES 10013
#define WFP_WSAEWOULDBLOCK 10035
#define WFP_WSAECONNREFUSED 10061

/* Classifier outcomes. */
#define WFP_VERDICT_DENIED 1      /* explicit ALE block (WSAEACCES) */
#define WFP_VERDICT_SILENT_DROP 2 /* no verdict, no answer (select timeout) */
#define WFP_VERDICT_CONNECTED 3   /* control connected: no enforcement */
#define WFP_VERDICT_REFUSED 4     /* fast refuse: SYN answered, no ALE block */
#define WFP_VERDICT_OTHER_ERROR 5 /* anything else: diagnose, never pass */

/* Classify one nonblocking TCP control attempt:
 *   sync_error   WSA error from connect(), or 0 when connect() succeeded.
 *   select_ready select() result after WSAEWOULDBLOCK (<=0 timeout/error,
 *                >0 writable); ignored unless sync_error is WSAEWOULDBLOCK.
 *   so_error     SO_ERROR after a writable select; ignored otherwise. */
static int wfp_classify_tcp_verdict(int sync_error, int select_ready, int so_error) {
    if (sync_error == WFP_WSAEACCES)
        return WFP_VERDICT_DENIED;
    if (sync_error != WFP_WSAEWOULDBLOCK) {
        if (sync_error == 0)
            return WFP_VERDICT_CONNECTED;
        if (sync_error == WFP_WSAECONNREFUSED)
            return WFP_VERDICT_REFUSED;
        return WFP_VERDICT_OTHER_ERROR;
    }
    if (select_ready <= 0)
        return WFP_VERDICT_SILENT_DROP;
    if (so_error == WFP_WSAEACCES)
        return WFP_VERDICT_DENIED;
    if (so_error == 0)
        return WFP_VERDICT_CONNECTED;
    if (so_error == WFP_WSAECONNREFUSED)
        return WFP_VERDICT_REFUSED;
    return WFP_VERDICT_OTHER_ERROR;
}

#endif
